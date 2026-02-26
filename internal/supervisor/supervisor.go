package supervisor

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Instances    []Instance     `json:"instances"`
	Restart      bool           `json:"restart"`
	RestartDelay DurationString `json:"restartDelay"`
	FailFast     bool           `json:"failFast"`
}

type Instance struct {
	Name     string            `json:"name"`
	Args     []string          `json:"args"`
	Env      map[string]string `json:"env"`
	Disabled bool              `json:"disabled"`
	WorkDir  string            `json:"workDir"`
}

type DurationString time.Duration

func (d *DurationString) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*d = 0
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		if strings.TrimSpace(s) == "" {
			*d = 0
			return nil
		}
		dd, err := time.ParseDuration(s)
		if err != nil {
			return err
		}
		*d = DurationString(dd)
		return nil
	}
	var secs float64
	if err := json.Unmarshal(b, &secs); err == nil {
		if secs < 0 {
			return fmt.Errorf("duration seconds must be >= 0")
		}
		*d = DurationString(time.Duration(secs * float64(time.Second)))
		return nil
	}
	return fmt.Errorf("invalid duration")
}

func (d DurationString) Duration(def time.Duration) time.Duration {
	if time.Duration(d) <= 0 {
		return def
	}
	return time.Duration(d)
}

func LoadConfig(path string) (Config, error) {
	var cfg Config
	if strings.TrimSpace(path) == "" {
		return cfg, fmt.Errorf("missing config path")
	}
	f, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return cfg, err
	}
	if len(cfg.Instances) == 0 {
		return cfg, fmt.Errorf("no instances configured")
	}
	seen := map[string]struct{}{}
	for i := range cfg.Instances {
		in := &cfg.Instances[i]
		in.Name = strings.TrimSpace(in.Name)
		if in.Name == "" {
			return cfg, fmt.Errorf("instances[%d].name required", i)
		}
		if _, ok := seen[in.Name]; ok {
			return cfg, fmt.Errorf("duplicate instance name %q", in.Name)
		}
		seen[in.Name] = struct{}{}
		if len(in.Args) == 0 {
			return cfg, fmt.Errorf("instances[%d].args required", i)
		}
	}
	return cfg, nil
}

func Run(ctx context.Context, configPath string) error {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load supervisor config: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}
	exe, _ = filepath.EvalSymlinks(exe)

	restartDelay := cfg.RestartDelay.Duration(2 * time.Second)
	failFast := cfg.FailFast
	if !cfg.Restart && !cfg.FailFast {
		failFast = true
	}
	log.Printf("supervisor: starting %d instance(s) restart=%t failFast=%t restartDelay=%s exe=%s", len(cfg.Instances), cfg.Restart, failFast, restartDelay, exe)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(cfg.Instances))
	var wg sync.WaitGroup
	started := 0
	for _, inst := range cfg.Instances {
		if inst.Disabled {
			log.Printf("supervisor: skipping disabled instance %q", inst.Name)
			continue
		}
		started++
		wg.Add(1)
		go func(inst Instance) {
			defer wg.Done()
			rErr := runInstanceLoop(ctx, exe, inst, cfg.Restart, restartDelay)
			if rErr != nil && !errors.Is(rErr, context.Canceled) {
				select {
				case errCh <- rErr:
				default:
				}
				if failFast {
					cancel()
				}
			}
		}(inst)
	}
	if started == 0 {
		return fmt.Errorf("no enabled instances")
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		<-done
		if len(errCh) > 0 {
			return <-errCh
		}
		return nil
	case err := <-errCh:
		cancel()
		<-done
		return err
	case <-done:
		if len(errCh) > 0 {
			return <-errCh
		}
		return nil
	}
}

func runInstanceLoop(ctx context.Context, exe string, inst Instance, restart bool, restartDelay time.Duration) error {
	for {
		err := runInstanceOnce(ctx, exe, inst)
		if !restart || ctx.Err() != nil {
			return err
		}
		if err == nil {
			// Child exited cleanly; do not spin forever unless restart is explicitly desired
			// for all exits. Current behavior: restart on any exit when restart=true.
		}
		log.Printf("supervisor[%s]: child exited (%v); restarting in %s", inst.Name, err, restartDelay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(restartDelay):
		}
	}
}

func runInstanceOnce(ctx context.Context, exe string, inst Instance) error {
	if err := ensureCatalogParentDirs(inst); err != nil {
		return fmt.Errorf("prepare instance dirs: %w", err)
	}
	cmd := exec.CommandContext(ctx, exe, inst.Args...)
	cmd.Env = mergedEnv(os.Environ(), inst.Env)
	if inst.WorkDir != "" {
		cmd.Dir = inst.WorkDir
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start child: %w", err)
	}
	log.Printf("supervisor[%s]: pid=%d args=%q", inst.Name, cmd.Process.Pid, strings.Join(inst.Args, " "))

	var ioWG sync.WaitGroup
	ioWG.Add(2)
	go func() {
		defer ioWG.Done()
		copyPrefixed(inst.Name, "stdout", stdout)
	}()
	go func() {
		defer ioWG.Done()
		copyPrefixed(inst.Name, "stderr", stderr)
	}()

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		_ = signalChild(cmd.Process)
		select {
		case err := <-waitCh:
			ioWG.Wait()
			if err != nil && !isExitBySignal(err) {
				return err
			}
			return ctx.Err()
		case <-time.After(8 * time.Second):
			_ = cmd.Process.Kill()
			<-waitCh
			ioWG.Wait()
			return ctx.Err()
		}
	case err := <-waitCh:
		ioWG.Wait()
		if err != nil {
			return fmt.Errorf("child exit: %w", err)
		}
		return nil
	}
}

func ensureCatalogParentDirs(inst Instance) error {
	for _, a := range inst.Args {
		if !strings.HasPrefix(a, "-catalog=") {
			continue
		}
		p := strings.TrimSpace(strings.TrimPrefix(a, "-catalog="))
		if p == "" {
			continue
		}
		// Child paths are often nested under /data/<bucket>/catalog.json in supervisor mode.
		dir := filepath.Dir(p)
		if dir == "." || dir == "/" || dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	return nil
}

func signalChild(p *os.Process) error {
	if p == nil {
		return nil
	}
	if err := p.Signal(os.Interrupt); err == nil {
		return nil
	}
	return nil
}

func isExitBySignal(err error) bool {
	var ee *exec.ExitError
	return errors.As(err, &ee)
}

func copyPrefixed(name, stream string, r io.Reader) {
	sc := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		log.Printf("[%s %s] %s", name, stream, sc.Text())
	}
	if err := sc.Err(); err != nil {
		log.Printf("[%s %s] read err=%v", name, stream, err)
	}
}

func mergedEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}
	idx := make(map[string]int, len(base))
	out := append([]string(nil), base...)
	for i, kv := range out {
		k, _, ok := strings.Cut(kv, "=")
		if ok {
			idx[k] = i
		}
	}
	for k, v := range overrides {
		kv := k + "=" + v
		if i, ok := idx[k]; ok {
			out[i] = kv
		} else {
			out = append(out, kv)
		}
	}
	return out
}

package vodwebdav

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/materializer"
	"github.com/snapetech/iptvtunerr/internal/vodfs"
	"golang.org/x/net/webdav"
)

const (
	methodPROPFIND   = "PROPFIND"
	readOnlyDAVAllow = "OPTIONS, PROPFIND, HEAD, GET"
)

func NewHandler(movies []catalog.Movie, series []catalog.Series, mat materializer.Interface) http.Handler {
	if mat == nil {
		mat = &materializer.Stub{}
	}
	base := &webdav.Handler{
		Prefix:     "/",
		FileSystem: &davFS{tree: vodfs.NewTree(movies, series), mat: mat},
		LockSystem: webdav.NewMemLS(),
		Logger:     func(*http.Request, error) {},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("MS-Author-Via", "DAV")
		switch r.Method {
		case http.MethodOptions, methodPROPFIND, http.MethodHead, http.MethodGet:
			base.ServeHTTP(w, r)
			return
		default:
			w.Header().Set("Allow", readOnlyDAVAllow)
			w.Header().Set("DAV", "1, 2")
			http.Error(w, "read-only WebDAV surface", http.StatusMethodNotAllowed)
			return
		}
	})
}

type davFS struct {
	tree *vodfs.Tree
	mat  materializer.Interface
}

func (d *davFS) Mkdir(context.Context, string, os.FileMode) error { return fs.ErrPermission }
func (d *davFS) RemoveAll(context.Context, string) error          { return fs.ErrPermission }
func (d *davFS) Rename(context.Context, string, string) error     { return fs.ErrPermission }
func (d *davFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	node, err := d.resolve(name)
	if err != nil {
		return nil, err
	}
	return node.info(), nil
}

func (d *davFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	if flag&(os.O_RDWR|os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		return nil, fs.ErrPermission
	}
	node, err := d.resolve(name)
	if err != nil {
		return nil, err
	}
	if node.isDir {
		return newDirFile(node), nil
	}
	return newLazyFile(ctx, node, d.mat), nil
}

type resolvedNode struct {
	name      string
	isDir     bool
	size      int64
	assetID   string
	streamURL string
	children  []os.FileInfo
}

func (n resolvedNode) info() os.FileInfo {
	mode := os.FileMode(0444)
	if n.isDir {
		mode = os.ModeDir | 0555
	}
	return fileInfo{name: n.name, size: n.size, mode: mode}
}

func (d *davFS) resolve(raw string) (*resolvedNode, error) {
	if d.tree == nil {
		return nil, fs.ErrNotExist
	}
	clean := path.Clean("/" + strings.TrimSpace(raw))
	switch clean {
	case "/", ".":
		return &resolvedNode{
			name:  "/",
			isDir: true,
			children: []os.FileInfo{
				fileInfo{name: "Movies", mode: os.ModeDir | 0555},
				fileInfo{name: "TV", mode: os.ModeDir | 0555},
			},
		}, nil
	case "/Movies":
		children := make([]os.FileInfo, 0, len(d.tree.Movies))
		for i := range d.tree.Movies {
			children = append(children, fileInfo{name: d.tree.MovieDirName(&d.tree.Movies[i]), mode: os.ModeDir | 0555})
		}
		return &resolvedNode{name: "Movies", isDir: true, children: children}, nil
	case "/TV":
		children := make([]os.FileInfo, 0, len(d.tree.Series))
		for i := range d.tree.Series {
			children = append(children, fileInfo{name: d.tree.ShowDirName(&d.tree.Series[i]), mode: os.ModeDir | 0555})
		}
		return &resolvedNode{name: "TV", isDir: true, children: children}, nil
	}

	parts := strings.Split(strings.TrimPrefix(clean, "/"), "/")
	if len(parts) >= 2 && parts[0] == "Movies" {
		movie, ok := d.tree.LookupMovieDir(parts[1])
		if !ok || movie == nil {
			return nil, fs.ErrNotExist
		}
		if len(parts) == 2 {
			filename := vodfs.MovieFileNameForStream(movie.Title, movie.Year, movie.StreamURL)
			return &resolvedNode{
				name:     parts[1],
				isDir:    true,
				children: []os.FileInfo{fileInfo{name: filename, size: placeholderSize(movie.StreamURL, 0), mode: 0444}},
			}, nil
		}
		filename := vodfs.MovieFileNameForStream(movie.Title, movie.Year, movie.StreamURL)
		if len(parts) == 3 && parts[2] == filename {
			return &resolvedNode{
				name:      filename,
				size:      placeholderSize(movie.StreamURL, 0),
				assetID:   movie.ID,
				streamURL: movie.StreamURL,
			}, nil
		}
		return nil, fs.ErrNotExist
	}
	if len(parts) >= 2 && parts[0] == "TV" {
		series, ok := d.tree.LookupShowDir(parts[1])
		if !ok || series == nil {
			return nil, fs.ErrNotExist
		}
		if len(parts) == 2 {
			children := make([]os.FileInfo, 0, len(series.Seasons))
			for _, se := range series.Seasons {
				children = append(children, fileInfo{name: vodfs.SeasonDirName(se.Number), mode: os.ModeDir | 0555})
			}
			return &resolvedNode{name: parts[1], isDir: true, children: children}, nil
		}
		var season *catalog.Season
		for i := range series.Seasons {
			if vodfs.SeasonDirName(series.Seasons[i].Number) == parts[2] {
				season = &series.Seasons[i]
				break
			}
		}
		if season == nil {
			return nil, fs.ErrNotExist
		}
		if len(parts) == 3 {
			children := make([]os.FileInfo, 0, len(season.Episodes))
			for _, ep := range season.Episodes {
				filename := vodfs.EpisodeFileNameForStream(series.Title, series.Year, ep.SeasonNum, ep.EpisodeNum, ep.Title, ep.StreamURL)
				children = append(children, fileInfo{name: filename, size: placeholderSize(ep.StreamURL, 0), mode: 0444})
			}
			return &resolvedNode{name: parts[2], isDir: true, children: children}, nil
		}
		if len(parts) == 4 {
			for _, ep := range season.Episodes {
				filename := vodfs.EpisodeFileNameForStream(series.Title, series.Year, ep.SeasonNum, ep.EpisodeNum, ep.Title, ep.StreamURL)
				if filename == parts[3] {
					return &resolvedNode{
						name:      filename,
						size:      placeholderSize(ep.StreamURL, 0),
						assetID:   ep.ID,
						streamURL: ep.StreamURL,
					}, nil
				}
			}
		}
	}
	return nil, fs.ErrNotExist
}

func placeholderSize(streamURL string, size int64) int64 {
	if size > 0 {
		return size
	}
	if strings.TrimSpace(streamURL) != "" {
		return 1
	}
	return 0
}

type fileInfo struct {
	name string
	size int64
	mode os.FileMode
}

func (f fileInfo) Name() string       { return f.name }
func (f fileInfo) Size() int64        { return f.size }
func (f fileInfo) Mode() os.FileMode  { return f.mode }
func (f fileInfo) ModTime() time.Time { return time.Time{} }
func (f fileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fileInfo) Sys() any           { return nil }

type dirFile struct {
	node resolvedNode
	pos  int
}

func newDirFile(node *resolvedNode) *dirFile { return &dirFile{node: *node} }

func (d *dirFile) Close() error                   { return nil }
func (d *dirFile) Read([]byte) (int, error)       { return 0, io.EOF }
func (d *dirFile) Write([]byte) (int, error)      { return 0, fs.ErrPermission }
func (d *dirFile) Seek(int64, int) (int64, error) { return 0, nil }
func (d *dirFile) Stat() (os.FileInfo, error)     { return d.node.info(), nil }
func (d *dirFile) Readdir(count int) ([]os.FileInfo, error) {
	if d.pos >= len(d.node.children) && count > 0 {
		return nil, io.EOF
	}
	if count <= 0 {
		out := d.node.children[d.pos:]
		d.pos = len(d.node.children)
		return out, nil
	}
	end := d.pos + count
	if end > len(d.node.children) {
		end = len(d.node.children)
	}
	out := d.node.children[d.pos:end]
	d.pos = end
	return out, nil
}

type lazyFile struct {
	ctx   context.Context
	node  resolvedNode
	mat   materializer.Interface
	local *os.File
}

func newLazyFile(ctx context.Context, node *resolvedNode, mat materializer.Interface) *lazyFile {
	return &lazyFile{ctx: ctx, node: *node, mat: mat}
}

func (f *lazyFile) ensureOpen() error {
	if f.local != nil {
		return nil
	}
	if f.mat == nil {
		return materializer.ErrNotReady{AssetID: f.node.assetID}
	}
	localPath, err := f.mat.Materialize(f.ctx, f.node.assetID, f.node.streamURL)
	if err != nil {
		return err
	}
	if strings.TrimSpace(localPath) == "" {
		return materializer.ErrNotReady{AssetID: f.node.assetID}
	}
	handle, err := os.Open(localPath)
	if err != nil {
		return err
	}
	f.local = handle
	return nil
}

func (f *lazyFile) Close() error {
	if f.local != nil {
		return f.local.Close()
	}
	return nil
}

func (f *lazyFile) Read(p []byte) (int, error) {
	if err := f.ensureOpen(); err != nil {
		return 0, err
	}
	return f.local.Read(p)
}

func (f *lazyFile) Seek(offset int64, whence int) (int64, error) {
	if err := f.ensureOpen(); err != nil {
		return 0, err
	}
	return f.local.Seek(offset, whence)
}

func (f *lazyFile) Write([]byte) (int, error) { return 0, fs.ErrPermission }
func (f *lazyFile) Readdir(int) ([]os.FileInfo, error) {
	return nil, errors.New("not a directory")
}

func (f *lazyFile) Stat() (os.FileInfo, error) {
	if f.local != nil {
		return f.local.Stat()
	}
	return f.node.info(), nil
}

func MountHint(goos, addr string) string {
	switch strings.ToLower(strings.TrimSpace(goos)) {
	case "darwin":
		return fmt.Sprintf("Finder -> Go -> Connect to Server -> http://%s/", addr)
	case "windows":
		return fmt.Sprintf("Use File Explorer 'Add a network location' or WebDAV client against http://%s/", addr)
	default:
		return fmt.Sprintf("Mount a WebDAV client against http://%s/", addr)
	}
}

func MountCommand(goos, addr, target string) string {
	addr = strings.TrimSpace(addr)
	target = strings.TrimSpace(target)
	switch strings.ToLower(strings.TrimSpace(goos)) {
	case "darwin":
		if target == "" {
			target = "/Volumes/IPTVTunerrVOD"
		}
		return fmt.Sprintf("mkdir -p %q && mount_webdav -S http://%s/ %q", target, addr, target)
	case "windows":
		if target == "" {
			target = "Z:"
		}
		return fmt.Sprintf("net use %s http://%s/ /persistent:no", target, addr)
	default:
		if target == "" {
			target = "/mnt/iptvtunerr-vod"
		}
		return fmt.Sprintf("gio mount dav://%s/  # or map with your preferred WebDAV client, mount point %s", addr, target)
	}
}

# On-demand Windows runner

Windows smoke coverage runs on the private `snapetech/packer` QEMU runner.

Use this label set for jobs that should wake a disposable Windows VM:

```yaml
runs-on: [self-hosted, Windows, X64, packer-windows]
```

The full `go test ./...` suite currently has Windows-specific failures around
POSIX file-mode assertions, so the smoke workflow builds the Windows binary and
runs a focused Windows-safe subset until those tests are made portable.

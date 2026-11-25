module github.com/fiberfx/ecd-runtime/apps

go 1.27.4

require (
	github.com/fiberfx/ecd-id/go v0.11.0
	github.com/fiberfx/ecd-runtime/go/ecr v0.7.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/fiberfx/ecd-runtime/go v0.7.0 // indirect
	github.com/fiberfx/ecd-runtime/go/sqlite v0.7.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/exp v0.0.0-20250305212735-054e65f0b394 // indirect
	golang.org/x/sys v0.47.0 // indirect
	modernc.org/libc v1.62.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.9.1 // indirect
	modernc.org/sqlite v1.37.0 // indirect
)

replace (
	github.com/fiberfx/ecd-runtime/go => ../go
	github.com/fiberfx/ecd-runtime/go/ecr => ../go/ecr
	github.com/fiberfx/ecd-runtime/go/sqlite => ../go/sqlite
)

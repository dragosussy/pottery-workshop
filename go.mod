module humidity

go 1.26.4

require (
	github.com/disintegration/imaging v1.6.2
	github.com/warthog618/go-gpiocdev v0.9.1
	golang.org/x/image v0.24.0
	golang.org/x/sys v0.49.0
	modernc.org/sqlite v1.61.0
)

require (
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/text v0.22.0 // indirect
	modernc.org/libc v1.79.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

// The frontend's npm dependencies may ship Go files; they aren't ours.
ignore ./web/node_modules

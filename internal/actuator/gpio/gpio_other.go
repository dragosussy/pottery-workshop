//go:build !linux

package gpio

// Output is unavailable off Linux.
type Output struct{}

// Open always fails off Linux.
func Open(name string, cfg Config) (*Output, error) { return nil, ErrUnsupported }

func (o *Output) Name() string      { return "" }
func (o *Output) Set(on bool) error { return ErrUnsupported }
func (o *Output) Close() error      { return nil }

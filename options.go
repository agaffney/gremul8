package gremul8

type Option func(*Machine)

func WithDisplay(d Display) Option {
	return func(m *Machine) {
		if d != nil {
			m.disp = d
		}
	}
}

package gremul8

import "math/rand/v2"

type Option func(*Machine)

func WithDisplay(d Display) Option {
	return func(m *Machine) {
		if d != nil {
			m.disp = d
		}
	}
}

func WithRng(r *rand.Rand) Option {
	return func(m *Machine) {
		if r != nil {
			m.rng = r
		}
	}
}

func WithAutoTick(autoTick bool) Option {
	return func(m *Machine) {
		m.autoTick = autoTick
	}
}

func WithQuirks(quirks Quirks) Option {
	return func(m *Machine) {
		m.quirks = quirks
	}
}

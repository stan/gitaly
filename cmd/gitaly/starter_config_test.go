package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsSecure(t *testing.T) {
	for _, test := range []struct {
		name   string
		secure bool
	}{
		{"tcp", false},
		{"unix", false},
		{"tls", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			conf := starterConfig{name: test.name}
			require.Equal(t, test.secure, conf.isSecure())
		})
	}
}

func TestFamily(t *testing.T) {
	for _, test := range []struct {
		name, family string
	}{
		{"tcp", "tcp"},
		{"unix", "unix"},
		{"tls", "tcp"},
	} {
		t.Run(test.name, func(t *testing.T) {
			conf := starterConfig{name: test.name}
			require.Equal(t, test.family, conf.family())
		})
	}
}

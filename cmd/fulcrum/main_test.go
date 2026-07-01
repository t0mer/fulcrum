package main

import (
	"reflect"
	"testing"
)

func TestWithoutServiceFlag(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
	}{
		{[]string{"--service", "install", "--server.port", "9000"}, []string{"--server.port", "9000"}},
		{[]string{"--service=install", "--config", "/etc/fulcrum.yaml"}, []string{"--config", "/etc/fulcrum.yaml"}},
		{[]string{"--server.port", "8080"}, []string{"--server.port", "8080"}},
		{[]string{"--service", "start"}, []string{}},
	}
	for _, c := range cases {
		got := withoutServiceFlag(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("withoutServiceFlag(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

package main

import (
	"reflect"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestParseDBusValue(t *testing.T) {
	tests := []struct {
		value string
		want  []interface{}
	}{
		{"", nil},
		{"hello", []interface{}{"hello"}},
		{"http://example.com", []interface{}{"http://example.com"}},
		{"string:int32:4", []interface{}{"int32:4"}},
		{"int32:-4", []interface{}{int32(-4)}},
		{"uint32:4", []interface{}{uint32(4)}},
		{"int64:4", []interface{}{int64(4)}},
		{"uint64:4", []interface{}{uint64(4)}},
		{"int16:4", []interface{}{int16(4)}},
		{"uint16:4", []interface{}{uint16(4)}},
		{"byte:255", []interface{}{byte(255)}},
		{"double:0.5", []interface{}{0.5}},
		{"boolean:true", []interface{}{true}},
		{"objpath:/org/example", []interface{}{dbus.ObjectPath("/org/example")}},
	}
	for _, tt := range tests {
		got, err := parseDBusValue(tt.value)
		if err != nil {
			t.Errorf("parseDBusValue(%q): %v", tt.value, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseDBusValue(%q) = %#v, want %#v", tt.value, got, tt.want)
		}
	}

	for _, value := range []string{"int32:x", "byte:256", "boolean:maybe"} {
		if _, err := parseDBusValue(value); err == nil {
			t.Errorf("parseDBusValue(%q): expected an error", value)
		}
	}
}

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"pavucontrol", []string{"pavucontrol"}},
		{"some_command --with-parameters", []string{"some_command", "--with-parameters"}},
		{"a  b\tc", []string{"a", "b", "c"}},
		{`notify-send 'hello world'`, []string{"notify-send", "hello world"}},
		{`notify-send "hello world" now`, []string{"notify-send", "hello world", "now"}},
		{`echo "it's" 'say "hi"'`, []string{"echo", "it's", `say "hi"`}},
		{`echo a\ b`, []string{"echo", "a b"}},
		{`echo "a \"b\""`, []string{"echo", `a "b"`}},
		{`echo 'a\b'`, []string{"echo", `a\b`}},
		{`echo ""`, []string{"echo", ""}},
		{`echo --opt="a b"`, []string{"echo", "--opt=a b"}},
	}
	for _, tt := range tests {
		got, err := splitCommand(tt.cmd)
		if err != nil {
			t.Errorf("splitCommand(%q): %v", tt.cmd, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitCommand(%q) = %q, want %q", tt.cmd, got, tt.want)
		}
	}

	for _, cmd := range []string{`echo 'a`, `echo "a`, `echo a\`} {
		if _, err := splitCommand(cmd); err == nil {
			t.Errorf("splitCommand(%q): expected an error", cmd)
		}
	}
}

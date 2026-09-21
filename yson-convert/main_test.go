package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrettyFormat(t *testing.T) {
	expected := `{
    "foo" = {
        "bar" = <
            "q" = "e";
        > %true;
        "baz" = "qqq";
    };
}`

	actual, err := handle([]byte("{foo={bar=<q=e>%true;baz=qqq}}"), "pretty", "pretty", false)

	if err != nil {
		t.Errorf("Should not produce an error")
	}

	if expected != actual {
		t.Errorf("Result was incorrect, got: %s, want: %s.", actual, expected)
	}
}

func TestFloat(t *testing.T) {
	expected := `1234`

	actual, err := handle([]byte("1234"), "j2y", "compact", false)

	if err != nil {
		t.Errorf("Should not produce an error")
	}

	if expected != actual {
		t.Errorf("Result was incorrect, got: %s, want: %s.", actual, expected)
	}
}

func TestSeekSingle(t *testing.T) {
	expected := 4
	actual, err := seek([]byte("1234"), "pretty")
	if err != nil {
		t.Errorf("Should not produce an error, got: %v", err)
	}
	if expected != actual {
		t.Errorf("Result was incorrect, got: %d, want: %d.", actual, expected)
	}
}

func TestSeekMulti(t *testing.T) {
	expected := 5
	actual, err := seek([]byte("1234 5678"), "pretty")
	if err != nil {
		t.Errorf("Should not produce an error, got: %v", err)
	}
	if expected != actual {
		t.Errorf("Result was incorrect, got: %d, want: %d.", actual, expected)
	}
}

func TestSeekMany(t *testing.T) {
	expected := 2
	actual, err := seek([]byte("1 2 3 4"), "pretty")
	if err != nil {
		t.Errorf("Should not produce an error, got: %v", err)
	}
	if expected != actual {
		t.Errorf("Result was incorrect, got: %d, want: %d.", actual, expected)
	}
}

func TestSeekObj(t *testing.T) {
	expected := 2
	actual, err := seek([]byte("{}{}"), "pretty")
	if err != nil {
		t.Errorf("Should not produce an error, got: %v", err)
	}
	if expected != actual {
		t.Errorf("Result was incorrect, got: %d, want: %d.", actual, expected)
	}
}

func TestCheckMode(t *testing.T) {
	cases := []struct {
		name       string
		mode       string
		format     string
		readAsSeq  bool
		wantTheErr bool
	}{
		{"guess", "guess", "pretty", false, false},
		{"seq with guess mode", "guess", "pretty", true, true},
		{"seq with y2j", "y2j", "compact", true, false},
		{"unknown mode", "bogus", "pretty", false, true},
		{"unknown format", "y2j", "bogus", false, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := checkMode(c.mode, c.format, c.readAsSeq)
			if c.wantTheErr && err == nil {
				t.Errorf("expected an error for mode %q format %q seq %v", c.mode, c.format, c.readAsSeq)
			}
			if !c.wantTheErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestReadInputArgument(t *testing.T) {
	data, err := readInput("", []string{"{a=1}"})
	if err != nil {
		t.Fatalf("Should not produce an error, got: %v", err)
	}
	if string(data) != "{a=1}" {
		t.Errorf("Result was incorrect, got: %s, want: %s.", data, "{a=1}")
	}
}

func TestReadInputFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "value.yson")
	if err := os.WriteFile(path, []byte("{a=1}"), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := readInput(path, nil)
	if err != nil {
		t.Fatalf("Should not produce an error, got: %v", err)
	}
	if string(data) != "{a=1}" {
		t.Errorf("Result was incorrect, got: %s, want: %s.", data, "{a=1}")
	}
}

func TestReadInputMissingFile(t *testing.T) {
	if _, err := readInput(filepath.Join(t.TempDir(), "absent.yson"), nil); err == nil {
		t.Error("expected an error for a file that does not exist")
	}
}

func TestReadInputRejectsArgumentWithFile(t *testing.T) {
	if _, err := readInput("value.yson", []string{"{a=1}"}); err == nil {
		t.Error("expected an error when a value argument and -i are both given")
	}
}

func TestReadInputRejectsTwoArguments(t *testing.T) {
	if _, err := readInput("", []string{"{a=1}", "{b=2}"}); err == nil {
		t.Error("expected an error for more than one value argument")
	}
}

// The output file is opened only after the input has been read and converted,
// which is what makes "-i f -o f" an edit in place rather than a truncation.
func TestWriteOutputInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.yson")
	if err := os.WriteFile(path, []byte("{foo={bar=1}}"), 0o640); err != nil {
		t.Fatal(err)
	}

	data, err := readInput(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := handle(data, prettifyMode, prettyFormat, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeOutput(path, result); err != nil {
		t.Fatalf("Should not produce an error, got: %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := result + "\n"; string(written) != want {
		t.Errorf("In place output was incorrect, got: %s, want: %s.", written, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o640 {
		t.Errorf("Permissions were incorrect, got: %o, want: 640.", perm)
	}
}

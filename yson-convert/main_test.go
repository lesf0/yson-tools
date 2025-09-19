package main

import "testing"

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
	actual := seek([]byte("1234"), "pretty")
	if expected != actual {
		t.Errorf("Result was incorrect, got: %d, want: %d.", actual, expected)
	}
}

func TestSeekMulti(t *testing.T) {
	expected := 5
	actual := seek([]byte("1234 5678"), "pretty")
	if expected != actual {
		t.Errorf("Result was incorrect, got: %d, want: %d.", actual, expected)
	}
}

func TestSeekObj(t *testing.T) {
	expected := 2
	actual := seek([]byte("{}{}"), "pretty")
	if expected != actual {
		t.Errorf("Result was incorrect, got: %d, want: %d.", actual, expected)
	}
}

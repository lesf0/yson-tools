package common

type Kind int

const (
	JSON Kind = iota
	YSON
)

func (f Kind) String() string {
	switch f {
	case JSON:
		return "JSON"
	case YSON:
		return "YSON"
	}

	panic("invalid kind")
}

func Chain(inputKind Kind, outputKind Kind, format Format) func([]byte) ([]byte, error) {
	var from func(s []byte) (any, error)
	var to func(any) ([]byte, error)

	switch inputKind {
	case JSON:
		from = ParseJson
	case YSON:
		from = ParseYson
	}

	switch outputKind {
	case JSON:
		to = format.JsonMarhaler()
	case YSON:
		to = format.YsonMarshaler()
	}

	return func(input []byte) ([]byte, error) {
		buf, err := from(input)
		if err != nil {
			return nil, err
		}
		return to(buf)
	}
}

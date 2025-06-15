package common

import (
	"go.ytsaurus.tech/yt/go/yson"
	"go.ytsaurus.tech/yt/go/yson/yson2json"
)

func ParseYson(s []byte) (any, error) {
	var ysonData any
	err := yson.Unmarshal(s, &ysonData)

	return ysonData, err
}

func ParseJson(s []byte) (any, error) {
	return yson2json.RawMessage{
		JSON:      s,
		UseInt64:  true,
		UseUint64: true,
	}, nil
}

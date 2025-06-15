package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/itchyny/gojq"
	"github.com/lesf0/yson-tools/pkg/common"
)

func main() {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		panic(err)
	}

	parsed, err := common.ParseYson(input)
	if err != nil {
		panic(err)
	}

	query, err := gojq.Parse(".")
	if err != nil {
		panic(err)
	}

	iter := query.Run(NormalizeYSON(parsed))
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			if err, ok := err.(*gojq.HaltError); ok && err.Value() == nil {
				break
			}
			log.Fatalln(err)
		}
		result, err := common.FormatPretty.YsonMarshaler()(DenormalizeYSON(v))
		if err != nil {
			panic(err)
		}
		fmt.Println(string(result))
	}
}

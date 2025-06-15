package main

import (
	"encoding/json"
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

	// gojq won't work directly with yson types
	jsonInput, err := common.Chain(common.YSON, common.JSON, common.FormatText)(input)
	if err != nil {
		panic(err)
	}

	var parsed any
	err = json.Unmarshal(jsonInput, &parsed)
	if err != nil {
		panic(err)
	}

	query, err := gojq.Parse(".[2].Attrs")
	if err != nil {
		panic(err)
	}

	normalizeQuery, err := gojq.Parse(`
	walk(
		if type == "object"
		then with_entries(
			if .key == "Attrs" then .key = "$attributes"
			elif .key == "Value" then .key = "$value"
			else . end
		) else . end
	)`)
	if err != nil {
		panic(err)
	}

	normalizer, err := gojq.Compile(normalizeQuery)
	if err != nil {
		panic(err)
	}

	iter := query.Run(parsed)
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

		normalizedIter := normalizer.Run(v)
		for {
			v, ok := normalizedIter.Next()
			if !ok {
				break
			}
			if err, ok := v.(error); ok {
				if err, ok := err.(*gojq.HaltError); ok && err.Value() == nil {
					break
				}
				log.Fatalln(err)
			}

			json, err := common.FormatText.JsonMarhaler()(v)
			if err != nil {
				panic(err)
			}
			yson, err := common.Chain(common.JSON, common.YSON, common.FormatPretty)(json)
			if err != nil {
				panic(err)
			}
			fmt.Println(string(yson))
		}
	}
}

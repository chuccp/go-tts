//go:build ignore

package main

import (
	"bufio"
	"compress/gzip"
	"flag"
	"fmt"
	"os"

	"github.com/ikawaha/kagome-dict/dict"
	"github.com/ikawaha/kagome-dict/ipa"
)

func main() {
	output := flag.String("output", "internal/tts/data/ipa_pronunciations.tsv.gz", "output path")
	flag.Parse()

	dictionary := ipa.Dict()
	pronunciationIndex, ok := dictionary.ContentsMeta[dict.PronunciationIndex]
	if !ok {
		panic("IPADIC pronunciation metadata is missing")
	}

	file, err := os.Create(*output)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	compressed := gzip.NewWriter(file)
	defer compressed.Close()
	writer := bufio.NewWriterSize(compressed, 1<<20)
	defer writer.Flush()

	rows := 0
	for id, content := range dictionary.Contents {
		index := int(pronunciationIndex) - len(dictionary.POSTable.POSs[id])
		if index < 0 || index >= len(content) {
			continue
		}
		pronunciation := content[index]
		if pronunciation == "" {
			continue
		}
		if _, err := fmt.Fprintf(writer, "%d\t%s\n", id, pronunciation); err != nil {
			panic(err)
		}
		rows++
	}
	if err := writer.Flush(); err != nil {
		panic(err)
	}
	if err := compressed.Close(); err != nil {
		panic(err)
	}
	if err := file.Close(); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %d pronunciations\n", rows)
}

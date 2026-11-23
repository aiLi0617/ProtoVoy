package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"protovoy.dev/protovoy/internal/project"
)

func importCurl(input io.Reader, output io.Writer) error {
	data, err := io.ReadAll(io.LimitReader(input, project.MaxCurlImportBytes+1))
	if err != nil {
		return errors.New("could not read the cURL command")
	}
	request, err := project.ParseCurl(string(data))
	if err != nil {
		return err
	}
	data, err = project.Encode(project.Project{Requests: []project.Request{request}})
	if err != nil {
		return err
	}
	_, err = output.Write(append(data, '\n'))
	return err
}

func runCurlCommand(args []string) {
	flags := flag.NewFlagSet("protovoy curl", flag.ExitOnError)
	index := flags.Int("request", 1, "one-based request number")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: protovoy curl [-request N] PROJECT.json")
		flags.PrintDefaults()
	}
	_ = flags.Parse(args)
	if flags.NArg() != 1 || *index < 1 {
		flags.Usage()
		os.Exit(2)
	}
	if err := writeCurlSnippet(flags.Arg(0), *index); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func writeCurlSnippet(path string, index int) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("could not open the collection file")
	}
	defer file.Close()
	value, err := project.Read(file)
	if err != nil {
		return err
	}
	if index > len(value.Requests) {
		return fmt.Errorf("request %d is outside the collection", index)
	}
	snippet, err := value.Requests[index-1].CurlSnippet(value.CollectionVariables)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, snippet)
	return err
}

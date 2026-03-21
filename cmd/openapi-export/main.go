package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/b42labs/northwatch/internal/openapi"
)

func main() {
	spec := openapi.BuildSpec()
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	_, _ = os.Stdout.Write(data)
	_, _ = fmt.Fprintln(os.Stdout)
}

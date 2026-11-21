package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"
)

// cmdHealth asks a running server whether it answers. Container images have
// no curl, so the program checks itself.
func cmdHealth(args []string) error {
	fs := flag.NewFlagSet("health", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	addr := fs.String("addr", "127.0.0.1:8080", "адреса serve")
	if err := fs.Parse(args); err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get("http://" + *addr + "/api/profiles")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("serve відповів %s", response.Status)
	}
	return nil
}

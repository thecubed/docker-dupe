package main

import (
	"github.com/jessevdk/go-flags"
	"github.com/thecubed/docker-dupe/dupe"
	"log"
	"os"
	"fmt"
)

const APP_NAME = "docker-dupe"
const APP_VERSION = "0.0.1"

var opts struct {
	Debug   bool `long:"debug" description:"Enable DEBUG logging"`
	DoVersion bool `short:"V" long:"version" description:"Print version and exit"`

	Source string `short:"s" long:"source" description:"Source docker registry URL" required:"true"`
	Destination string `short:"d" long:"dest" description:"Destination docker registry URL" required:"true"`

	ManifestName string `short:"n" long:"name" description:"Docker manifest name to pull from source" required:"true"`
	ManifestTag string `short:"t" long:"tag" description:"Docker manifest tag to pull from source" required:"true"`

	Concurrency int `short:"c" long:"concurrency" description:"Concurrent operation limit" default:"4"`
}

func main() {

	// Parse arguments
	_, err := flags.Parse(&opts)
	// From https://www.snip2code.com/Snippet/605806/go-flags-suggested--h-documentation
	if err != nil {
		typ := err.(*flags.Error).Type
		if typ == flags.ErrHelp {
			os.Exit(0)
		} else if !opts.DoVersion {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	// Print version number if requested from command line
	if opts.DoVersion == true {
		fmt.Printf("%s %s at your service.\n", APP_NAME, APP_VERSION)
		os.Exit(10)
	}

	// Create the dupe agent
	dupe := dupe.New(&dupe.DupeConfig{
		UrlFrom: opts.Source,
		UrlTo: opts.Destination,
		Threads: opts.Concurrency,
		Debug: opts.Debug,
	})

	// Copy the layers + manifest
	dupe.Copy(opts.ManifestName, opts.ManifestTag)

	// We done here. Go home
	log.Print("Finished!!!")

}





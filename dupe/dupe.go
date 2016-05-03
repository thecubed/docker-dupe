package dupe

import (
	"github.com/heroku/docker-registry-client/registry"
	"github.com/mitchellh/ioprogress"
	manifest "github.com/docker/distribution/manifest/schema1"
	//"github.com/docker/distribution/digest"
	//"github.com/docker/libtrust"
	"log"
	"fmt"
	"os"
	"sync"
	"io/ioutil"
)

type Dupe struct {
	// Docker image you want to copy
	manifestName string

	// Tag for the image to be copied
	manifestTag  string

	// The manifest data from the source
	manifest *manifest.SignedManifest

	// The source registry
	from         *registry.Registry

	// The destination registry
	to           *registry.Registry

	// Concurrency for the pull/push operation
	threads      int

	// Logger
	log          *log.Logger

	// Debug logger
	debug        *log.Logger

}

type DupeConfig struct {
	// Docker registry to pull images from, in http(s)://url.to.registry/ form
	UrlFrom      string
	// Docker registry to push to, in http(s)://url.to.registry/ form
	UrlTo        string

	// Username to auth to source registry
	UserFrom     string

	// Username to auth to dest registry
	UserTo       string

	// Password to auth to source registry
	PasswordFrom string

	// Password to auth to dest registry
	PasswordTo   string

	// Concurrency limit (threads) to use for pull/push operation.
	// Each layer is a goroutine, so you can make this as concurrent as you'd like
	// but I wouldn't recommend anything above 16 threads.
	Threads      int

	// Set this to true to get extra logging messages.
	Debug        bool
}

// Struct for the layer copy goroutines to use
type worker struct {
	// Layer index (for tracking)
	index int

	// The actual layer object to be copied
	layer manifest.FSLayer
}

// Create a new DockerDupe object.
// This requires a DupeConfig struct to be passed to it that contains the mandatory options
// such as registry names and thread limit.
// Once this object is created, you can call .Copy() on it to duplicate a manifest and layers.
func New(config *DupeConfig) *Dupe {
	var dupe Dupe
	var err error

	dupe.log = log.New(os.Stdout, "MSG:    ", log.Ldate|log.Ltime|log.Lshortfile)
	dupe.debug = log.New(os.Stdout, "DBG:    ", log.Ldate | log.Ltime | log.Lshortfile)

	// Connect to both registries
	dupe.log.Printf("Connecting to source registry %s", config.UrlFrom)
	dupe.from, err = registry.New(config.UrlFrom, config.UserFrom, config.PasswordFrom)
	if err != nil {
		dupe.log.Fatalf("Unable to connect to source registry %s with error: %s", config.UrlTo, err)
	}
	dupe.log.Printf("Connecting to destination registry %s", config.UrlTo)
	dupe.to, err = registry.New(config.UrlTo, config.UserTo, config.PasswordTo)
	if err != nil {
		dupe.log.Fatalf("Unable to connect to source registry %s with error: %s", config.UrlTo, err)
	}

	if !config.Debug {
		dupe.debug.SetFlags(0)
		dupe.debug.SetOutput(ioutil.Discard)
		dupe.from.Logf = registry.Quiet
		dupe.to.Logf = registry.Quiet
	}

	dupe.threads = config.Threads

	return &dupe
}

// Copy a manifest's layers and the manifest itself from the source docker registry to a destination registry.
// This uses goroutines to speed up the copy (a multithreaded copy is faster than not)
func (dupe *Dupe) Copy(manifest_name string, manifest_tag string) {
	var err error
	dupe.manifestName = manifest_name
	dupe.manifestTag = manifest_tag

	// Download the source manifest
	dupe.manifest, err = dupe.from.Manifest(dupe.manifestName, dupe.manifestTag)
	if err != nil {
		dupe.log.Fatalf("Unable to retrieve source manifest, error: %s", err)
	}

	dupe.debug.Print("Retrieved source manifest successfully.")

	// Create a copyLayer worker pool
	tasks := make(chan *worker, 64)
	var wg sync.WaitGroup

	// Spawn the workers and add them to the wait group
	for i := 0; i < dupe.threads; i++ {
		wg.Add(1)
		go func() {
			for task := range tasks {
				dupe.copyLayer(task.index, task.layer)
			}
			wg.Done()
		}()
	}

	// Upload each source layer to the destination
	for index, layer := range dupe.manifest.FSLayers {
		tasks <- &worker{
			index: index,
			layer: layer,
		}
	}
	close(tasks)

	// Wait here until all layers are finished uploading
	wg.Wait()

	// Upload the manifest
	dupe.log.Print("Uploading manifest...")
	dupe.copyManifest()

	// We're done! Party!
	dupe.log.Print("Upload complete!")
}

// Internal function to copy the manifest itself from the source to destination registry.
// We use the manifest exactly as it is on the source server to avoid the need to re-sign it,
// preserving the chain of trust for the docker image.
func (dupe *Dupe) copyManifest() {
	// Not needed, we dont' need to sign the manifest since we're directly copying it.

	//key, err := libtrust.GenerateECP256PrivateKey()
	//if err != nil {
	//	dupe.log.Fatalf("Unable to create manifest signing key. Error: %s", err)
	//}
	//signedManifest, err := manifest.Sign(dupe.manifest, key)
	//if err != nil {
	//	dupe.log.Fatalf("Unable to sign manifest. Error: %s", err)
	//}
	err := dupe.to.PutManifest(dupe.manifestName, dupe.manifestTag, dupe.manifest)
	if err != nil {
		dupe.log.Fatalf("Unable to upload manifest to destination registry. Error: %s", err)
	}
}

// Internal function to copy a layer from one registry to another.
// We open a io.Reader on the source and an io.Writer on the destination and wire the two together.
// In between is a progressbar function that gives us a nice pretty progress bar so we can tell the app isn't hung.
func (dupe *Dupe) copyLayer(index int, layer manifest.FSLayer) {
	dupe.debug.Printf("#%d: Downloading layer %s", index, layer.BlobSum)

	// Check if destination has this layer
	exists, err := dupe.to.HasLayer(dupe.manifestName, layer.BlobSum);
	if err != nil {
		// Failed to check if layer exists in dest, fail!
		dupe.log.Fatalf("Unable to check if layer exists in destination registry. Error: %s", err)
	}

	if !exists {
		// Destination registry doesn't have layer, upload it
		src_reader, src_len, err := dupe.from.DownloadLayerLength(dupe.manifestName, layer.BlobSum)
		if src_reader != nil {
			defer src_reader.Close()
		} else if err != nil {
			dupe.log.Fatalf("Unable to create reader, err: %s", err)
		} else {
			dupe.log.Fatalf("Reader was nil? %s", src_reader)
		}

		progress := func(progress, total int64) string {
			bar := ioprogress.DrawTextFormatBar(20)
			return fmt.Sprintf("Uploading layer %s : %s %s",
				layer.BlobSum,
				bar(progress, total),
				ioprogress.DrawTextFormatBytes(progress, total))
		}

		progress_reader := &ioprogress.Reader{
			Reader: src_reader,
			Size:   src_len,
			DrawFunc:     ioprogress.DrawTerminalf(os.Stderr, progress),
		}

		// Upload to dest
		dupe.to.UploadLayer(dupe.manifestName, layer.BlobSum, progress_reader)
	} else {
		// Layer exists, go to next one
		dupe.log.Printf("Skipping %s", layer.BlobSum)
	}
}


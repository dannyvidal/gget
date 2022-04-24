package main

import (
	"context"
	"errors"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/docker/distribution/uuid"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/ttacon/chalk"
)

//go:embed Dockerfile.tar.gz
var f embed.FS

// Write json response to stdout
type ErrorDetail struct {
	Message string `json:"message"`
}
type Aux struct {
	ID string `json:"ID"`
}
type DockerJSONWriter struct {
	Stream string `json:"stream"`
	Aux    Aux    `json:"aux"`

	ErrorDetail ErrorDetail `json:"errorDetail"`
}

func (d *DockerJSONWriter) TagExists(tag string) bool {
	return strings.Trim(tag, "\n") != ""
}
func (d *DockerJSONWriter) Print(phase string, r io.ReadCloser) error {

	j := json.NewDecoder(r)
	for err := j.Decode(d); err != io.EOF; err = j.Decode(d) {
		if err != nil {
			return err
		}

		switch phase {
		case "BUILD":
			if d.TagExists(d.Stream) {
				fmt.Printf("<%s> <%s> %s\n", chalk.Green.Color(phase), chalk.Yellow.Color("stream"), chalk.White.Color(d.Stream))
			}
			if d.TagExists(d.Aux.ID) {
				fmt.Printf("<%s> <%s> %s\n", chalk.Green.Color(phase), chalk.Yellow.Color("aux"), chalk.White.Color(d.Aux.ID))
			}
			if d.TagExists(d.ErrorDetail.Message) {
				fmt.Printf("<%s> <%s> %s\n", chalk.Red.Color(phase), chalk.Red.Color("error"), chalk.Underline.TextStyle(chalk.Red.Color(d.ErrorDetail.Message)))
			}
		}
	}
	return nil
}

type DockerImage struct {
	ID          string
	SourceDir 	string
	URL 		string
	ContextRoot context.Context
	Client      *client.Client
	JSON        *DockerJSONWriter
}

func (di *DockerImage) CreateContainer(ctxroot context.Context, chID chan string) error {
	defer close(chID)
	body, err := di.Client.ContainerCreate(
		ctxroot,
		&container.Config{
			Image:        di.ID,
			AttachStdout: true,
			AttachStderr: true,
			Entrypoint:   []string{"git-dumper", di.URL, "/git"},
		},
		&container.HostConfig{
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeBind,
					Source: di.SourceDir,
					Target: "/git",
				},
			},
		},
		&network.NetworkingConfig{},
		&v1.Platform{
			OS: "linux",
		},
		//random uuid string for docker container name
		uuid.Generate().String(),
	)


	if err != nil {
		return err
	}

	chID <- body.ID
	return nil
}
func (di *DockerImage) RunContainer(ctxroot context.Context, id string) error {
	fmt.Printf("<%s> <%s> %s\n", chalk.Green.Color("RUN"), chalk.Yellow.Color("ID"), chalk.White.Color("Running container "+id))

	err := di.Client.ContainerStart(ctxroot, id, types.ContainerStartOptions{})
	if err != nil {
		return err
	}
	rc, err := di.Client.ContainerLogs(ctxroot, id, types.ContainerLogsOptions{
		Follow:     true,
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return err
	}
	io.Copy(os.Stdout, rc)
	di.Client.ContainerRemove(ctxroot, id, types.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	})

	if err != nil {
		return err
	}
	return nil
}

// builds from embedded dockerfile
func NewDockerImage(ctxroot context.Context, url string, sourcedir string) (*DockerImage, error) {
	client, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Fatal(err)
	}
	data, err := f.Open("Dockerfile.tar.gz")

	if err != nil {
		return nil, err
	}

	img := DockerImage{
		Client: client,
		ContextRoot: ctxroot,
		JSON: &DockerJSONWriter{},
		URL: url,
		SourceDir: sourcedir,
	 }

	resp, err := client.ImageBuild(ctxroot, data, types.ImageBuildOptions{SuppressOutput: false})
	if err != nil {
		return nil, err
	}
	err = img.JSON.Print("BUILD", resp.Body)
	img.ID = strings.Split(img.JSON.Aux.ID, ":")[1]
	if err != nil {
		return nil, err
	}
	return &img, nil
}

func ConfigureFlags(url *string, output *string){
	if *url == "" {
		log.Fatal(errors.New("output directory must be specified"))
	}

	if *output == "" {
		log.Fatal(errors.New("output directory must be specified"))
	}

	if strings.Contains(*output, "~") {
		if homeDir, err := os.UserHomeDir(); err != nil {
			log.Fatal(err)
		} else {
			*output = strings.Replace(*output, "~", homeDir, 1)
		}
	}
	if !path.IsAbs(*output) {
		if absp, err := filepath.Abs(*output); err != nil {
			log.Fatal(err)
		} else {
			fmt.Println(absp)
			*output = absp
		}
	}
	err := os.MkdirAll(*output, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}

}

func main() {
	var (
		output string
		url    string
	)
	flag.StringVar(&output, "o", "", "-o \"Some Output Directory\"")
	flag.StringVar(&url, "u", "", "-u \"Some .git URL\"")
	flag.Parse()
	ConfigureFlags(&url, &output)

	ctxroot := context.Background()
	chID := make(chan string, 1)
	img, err := NewDockerImage(ctxroot, url, output)

	if err != nil {
		log.Fatal(err)
	}

	err = img.CreateContainer(ctxroot, chID)

	if err != nil {
		log.Fatal(err)
	}
	id := <-chID
	err = img.RunContainer(ctxroot, id)

	if err != nil {
		log.Fatal(err)
	}
}

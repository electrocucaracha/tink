// Copyright © 2018 packet.net

package hardware

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinkerbell/tink/client"
	"github.com/tinkerbell/tink/pkg"
	hwpb "github.com/tinkerbell/tink/protos/hardware"
)

var (
	file  string
	sFile = "file"
)

// pushCmd represents the push command.
func NewPushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "push new hardware to tink",
		Example: `cat /tmp/data.json | tink hardware push
tink hardware push --file /tmp/data.json`,
		PreRunE: func(c *cobra.Command, args []string) error {
			if !isInputFromPipe() {
				path, _ := c.Flags().GetString(sFile)
				if path == "" {
					return fmt.Errorf("either pipe the data or provide the required '--file' flag")
				}
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			var data string
			var err error

			if isInputFromPipe() {
				data = readDataFromStdin()
			} else {
				data, err = readDataFromFile()
				if err != nil {
					log.Fatalf("read data from file failed: %v", err)
				}
			}
			s := struct {
				ID string
			}{}
			if json.NewDecoder(strings.NewReader(data)).Decode(&s) != nil {
				log.Fatalf("invalid json: %s", data)
			} else if s.ID == "" {
				log.Fatalf("invalid json, ID is required: %s", data)
			}

			var hw pkg.HardwareWrapper
			err = json.Unmarshal([]byte(data), &hw)
			if err != nil {
				log.Fatal(err)
			}
			if _, err := client.HardwareClient.Push(context.Background(), &hwpb.PushRequest{Data: hw.Hardware}); err != nil {
				log.Fatal(err)
			}
			log.Println("Hardware data pushed successfully")
		},
	}
	flags := cmd.PersistentFlags()
	flags.StringVarP(&file, "file", "", "", "hardware data file")
	return cmd
}

func isInputFromPipe() bool {
	fileInfo, _ := os.Stdin.Stat()
	return fileInfo.Mode()&os.ModeCharDevice == 0
}

func readDataFromStdin() string {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return ""
	}
	return string(data)
}

func readDataFromFile() (string, error) {
	f, err := os.Open(filepath.Clean(file))
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

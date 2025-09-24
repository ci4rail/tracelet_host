/*
Copyright © 2024 Ci4Rail GmbH
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/ci4rail/tracelet_host/devsim/internal/tracelet"
	"github.com/ci4rail/tracelet_host/devsim/pkg/version"
	"github.com/spf13/cobra"
)

var (
	deviceID              string
	locationServerAddress string
	httpsPort             int = 443
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "devsim",
	Short: "tracelet simulator",
	Long:  `Simulate a tracelet like SIO02`,
	Run: func(cmd *cobra.Command, args []string) {
		log.Printf("devsim version: %s\n", version.Version)

		_, err := tracelet.NewInstance(deviceID, locationServerAddress, httpsPort)

		if err != nil {
			log.Fatalf("Failed to create tracelet instance: %s", err)
		}
		select {} // wait until abort
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&deviceID, "dev-id", "d", "devsim", "device id to use")
	rootCmd.PersistentFlags().StringVarP(&locationServerAddress, "loc-srv", "l", "127.0.0.1:11002", "IP address of location server with port")
	rootCmd.PersistentFlags().IntVarP(&httpsPort, "https-port", "p", 443, "HTTPS port to use")	
}

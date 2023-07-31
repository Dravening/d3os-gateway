/*
Copyright 2017 The Kubernetes Authors.

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

package cmd

import (
	"fmt"
	"net/http"

	"d3os-gateway/pkg/controller"

	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

const (
	LONG_VERSION  = "20230725,use coreDNS to route"
	SHORT_VERSION = "0725"
)

func newVersionCommand() *cobra.Command {
	var long bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "version for d3os-ingress-controller",
		Run: func(cmd *cobra.Command, _ []string) {
			if long {
				fmt.Print(LONG_VERSION)
			} else {
				fmt.Printf("d3os-ingress-controller version %s\n", SHORT_VERSION)
			}
		},
	}

	cmd.PersistentFlags().BoolVar(&long, "long", false, "show long mode version information")
	return cmd
}

func NewIngressCommand() *cobra.Command {
	var (
		kubeconfig       string
		masterAddr       string
		ingressClassName string
		httpAddr         string
	)

	cmd := &cobra.Command{
		Use:  "ingress [flags]",
		Long: `launch the ingress controller`,
		Run: func(cmd *cobra.Command, args []string) {

			// creates the connection
			config, err := clientcmd.BuildConfigFromFlags(masterAddr, kubeconfig)
			if err != nil {
				klog.Fatal(err)
			}

			contr := controller.NewD3osGatewayController(config, ingressClassName)

			// Now let's start the controller
			stop := make(chan struct{})
			defer close(stop)
			go contr.Run(2, stop)

			klog.Infof("[gateway web start backend]")
			webRunFunc := func() error {
				http.HandleFunc("/", contr.HandleRequestAndRedirect)
				srv := http.Server{Addr: httpAddr}
				err := srv.ListenAndServe()
				if err != nil {
					klog.Errorf("[metrics.web.error][err:%v]", err)
				}
				return err
			}
			errChan := make(chan error, 1)
			go func() {
				errChan <- webRunFunc()
			}()
			select {
			case err := <-errChan:
				klog.Errorf("[web.server.error][err:%v]", err)
				return
			case <-stop:
				klog.Info("receive.quit.signal.web.server.exit")
				return
			}
		},
	}
	cmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig-path", "kubeconfig", "absolute path to the kubeconfig file")
	cmd.PersistentFlags().StringVar(&masterAddr, "master", "", "master url")
	cmd.PersistentFlags().StringVar(&ingressClassName, "ingressClassName", "d3os", "unique name of the controller")
	cmd.PersistentFlags().StringVar(&httpAddr, "httpAddr", "0.0.0.0:80", "gateway api addr")

	return cmd
}

// NewD3OSIngressControllerCommand creates the d3os-ingress-controller command.
func NewD3OSIngressControllerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "d3os-ingress-controller [command]",
		Long:    "Yet another Ingress controller for Kubernetes using Apache d3os as the high performance reverse proxy.",
		Version: SHORT_VERSION,
	}

	cmd.AddCommand(NewIngressCommand())
	cmd.AddCommand(newVersionCommand())
	return cmd
}

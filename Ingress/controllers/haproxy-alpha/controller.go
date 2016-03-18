/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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
	"log"
	"os"
	"os/exec"
	"reflect"
	"text/template"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
	client "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/util"
)

// ingress encapsulates a single backend entry in the load balancer config.

type staticPageHandler struct {
	pagePath     string
	pageContents []byte
	c            *http.Client
}

// Get serves the error page
func (s *staticPageHandler) Getfunc(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(404)
	w.Write(s.pageContents)
}

func shellOut(cmd string) {
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to execute %v: %v, err: %v", cmd, string(out), err)
	}
}

func restartHaproxy(cmd string) {

	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		log.Printf("Failed to parse haproxy conf %v: %v, err: %v", cmd, string(out), err)
		log.Printf("Not restarting haproxy.")
	} else {
		log.Printf("Haproxy config looks good.  Restarting haproxy")
		shellOut("haproxy_reload")
	}
}

func main() {
	flags.Parse(os.Args)
	cfg := parseCfg(*config, *lbDefAlgorithm, *sslCert, *sslCaCert)

	defErrorPage := newStaticPageHandler(*errorPage, defaultErrorPage)
	if defErrorPage == nil {
		glog.Fatalf("Failed to load the default error page")
	}

	if *startSyslog {
		cfg.startSyslog = true
		_, err = newSyslogServer("/var/run/haproxy.log.socket")
		if err != nil {
			glog.Fatalf("Failed to start syslog server: %v", err)
		}
	}
	go registerHandlers(defErrorPage)
	var ingClient client.IngressInterface
	if kubeClient, err := client.NewInCluster(); err != nil {
		log.Fatalf("Failed to create client: %v.", err)
	} else {
		ingClient = kubeClient.Extensions().Ingress(os.Getenv("INGRESS_NAMESPACE"))
	}
	tmpl, _ := template.New("haproxy").Parse("template.cfg")
	rateLimiter := util.NewTokenBucketRateLimiter(0.1, 1)
	known := &extensions.IngressList{}

	// Controller loop
	shellOut("haproxy")
	for {
		rateLimiter.Accept()
		ingresses, err := ingClient.List(api.ListOptions{})
		if err != nil {
			log.Printf("Error retrieving ingresses: %v", err)
			continue
		}
		if reflect.DeepEqual(ingresses.Items, known.Items) {
			continue
		}
		known = ingresses
		if w, err := os.Create("/etc/haproxy/haproxy.conf"); err != nil {
			log.Fatalf("Failed to create haproxy.conf : %v", err)
		} else if err := tmpl.Execute(w, ingresses); err != nil {
			log.Fatalf("Failed to write template %v", err)
		}

		restartHaproxy("haproxy -c")

	}
}

// HAPROXY SERVICE LOAD BALANCER MAIN FUNC
func main() {
	clientConfig := kubectl_util.DefaultClientConfig(flags)
	flags.Parse(os.Args)
	cfg := parseCfg(*config, *lbDefAlgorithm, *sslCert, *sslCaCert)

	var kubeClient *unversioned.Client
	var err error

	defErrorPage := newStaticPageHandler(*errorPage, defaultErrorPage)
	if defErrorPage == nil {
		glog.Fatalf("Failed to load the default error page")
	}

	go registerHandlers(defErrorPage)

	var tcpSvcs map[string]int
	if *tcpServices != "" {
		tcpSvcs = parseTCPServices(*tcpServices)
	} else {
		glog.Infof("No tcp/https services specified")
	}

	if *startSyslog {
		cfg.startSyslog = true
		_, err = newSyslogServer("/var/run/haproxy.log.socket")
		if err != nil {
			glog.Fatalf("Failed to start syslog server: %v", err)
		}
	}
}

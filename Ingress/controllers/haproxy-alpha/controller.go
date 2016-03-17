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
type service struct {
	Name string
	Ep   []string

	// Kubernetes endpoint port. The application must serve a 200 page on this port.
	BackendPort int

	// FrontendPort is the port that the loadbalancer listens on for traffic
	// for this service. For http, it's always :80, for each tcp service it
	// is the service port of any service matching a name in the tcpServices set.
	FrontendPort int

	// Host if not empty it will add a new haproxy acl to route traffic using the
	// host header inside the http request. It only applies to http traffic.
	Host string

	// if true, terminate ssl using the loadbalancers certificates.
	SslTerm bool

	// if set use this to match the path rule
	AclMatch string

	// Algorithm
	Algorithm string

	// If SessionAffinity is set and without CookieStickySession, requests are routed to
	// a backend based on client ip. If both SessionAffinity and CookieStickSession are
	// set, a SERVERID cookie is inserted by the loadbalancer and used to route subsequent
	// requests. If neither is set, requests are routed based on the algorithm.

	// Indicates if the service must use sticky sessions
	// http://cbonte.github.io/haproxy-dconv/configuration-1.5.html#stick-table
	// Enabled using the attribute service.spec.sessionAffinity
	// https://github.com/kubernetes/kubernetes/blob/master/docs/user-guide/services.md#virtual-ips-and-service-proxies
	SessionAffinity bool

	// CookieStickySession use a cookie to enable sticky sessions.
	// The name of the cookie is SERVERID
	// This only can be used in http services
	CookieStickySession bool
}

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
	var ingClient client.IngressInterface
	if kubeClient, err := client.NewInCluster(); err != nil {
		log.Fatalf("Failed to create client: %v.", err)
	} else {
		ingClient = kubeClient.Extensions().Ingress("devops-test")
	}
	tmpl, _ := template.New("nginx").Parse(nginxConf)
	rateLimiter := util.NewTokenBucketRateLimiter(0.1, 1)
	known := &extensions.IngressList{}

	// Controller loop
	shellOut("nginx")
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
		if w, err := os.Create("/etc/nginx/nginx.conf"); err != nil {
			log.Fatalf("Failed to open %v: %v", nginxConf, err)
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

	if *cluster {
		if kubeClient, err = unversioned.NewInCluster(); err != nil {
			glog.Fatalf("Failed to create client: %v", err)
		}
	} else {
		config, err := clientConfig.ClientConfig()
		if err != nil {
			glog.Fatalf("error connecting to the client: %v", err)
		}
		kubeClient, err = unversioned.New(config)
	}
	namespace, specified, err := clientConfig.Namespace()
	if err != nil {
		glog.Fatalf("unexpected error: %v", err)
	}
	if !specified {
		namespace = api.NamespaceAll
	}

	// TODO: Handle multiple namespaces
	lbc := newLoadBalancerController(cfg, kubeClient, namespace, tcpSvcs)

	go lbc.epController.Run(util.NeverStop)
	go lbc.svcController.Run(util.NeverStop)
	if *dry {
		dryRun(lbc)
	} else {
		lbc.cfg.reload()
		util.Until(lbc.worker, time.Second, util.NeverStop)
	}

}

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

const (
	haproxyConf = `
global
    daemon
    stats socket /tmp/haproxy
    server-state-file global       
    server-state-base /var/state/haproxy/
		quiet
		nbproc  2
		pidfile /var/run/haproxy-private.pid

defaults
    log global
   
    load-server-state-from-file global
    
    # Enable session redistribution in case of connection failure.
    option redispatch
    
    # Disable logging of null connections (haproxy connections like checks). 
    # This avoids excessive logs from haproxy internals.
    option dontlognull
    
    # Enable HTTP connection closing on the server side.
    option http-server-close

    # Enable insertion of the X-Forwarded-For header to requests sent to 
    # servers and keep client IP address.
    option forwardfor
    
    # Enable HTTP keep-alive from client to server.
    option http-keep-alive

    # Clients should send their full http request in 5s.
    timeout http-request    5s
    
    # Maximum time to wait for a connection attempt to a server to succeed.
    timeout connect         5s

    # Maximum inactivity time on the client side.
    # Applies when the client is expected to acknowledge or send data.
    timeout client          50s

    # Inactivity timeout on the client side for half-closed connections.
    # Applies when the client is expected to acknowledge or send data 
    # while one direction is already shut down.
    timeout client-fin      50s
    
    # Maximum inactivity time on the server side.
    timeout server          50s
    
    # timeout to use with WebSocket and CONNECT
    timeout tunnel          1h
    
    # Maximum allowed time to wait for a new HTTP request to appear.
    timeout http-keep-alive 60s

    # default traffic mode is http
    # mode is overwritten in case of tcp services
    mode http

    # default default_backend. This allows custom default_backend in frontends
    default_backend default-backend

backend default-backend
  server localhost 127.0.0.1:8081

# haproxy stats, required hostport and firewall rules for :1936
listen stats
    bind *:1936
    stats enable
    stats hide-version
    stats realm Haproxy\ Statistics
    stats uri /

frontend httpfrontend
    # Frontend bound on all network interfaces on port 80
    bind *:80

{{range $ing := .Items}}
{{range $rule := $ing.Spec.Rules}}
backend {{$rule.Host}};
    option  httplog
    errorfile 400 /etc/haproxy/errors/400.http
    errorfile 403 /etc/haproxy/errors/403.http
    errorfile 408 /etc/haproxy/errors/408.http
    errorfile 500 /etc/haproxy/errors/500.http
    errorfile 502 /etc/haproxy/errors/502.http
    errorfile 503 /etc/haproxy/errors/503.http
    errorfile 504 /etc/haproxy/errors/504.http
    {{ range $path := $rule.HTTP.Paths }}
    acl url_acl_{{$rule.Host}} path_beg /{{$path.Path}}
    {{ if $svc.Host }}acl host_acl_{{$rule.Host}} hdr(host) {{$svc.Host}}
    use_backend {{$rule.Host}} if url_acl_{{$rule.Host}} or host_acl_{{$rule.Host}}
    {{ else }}use_backend {{$rule.Host}} if url_acl_{{$rule.Host}}
{{ end }}
{{end}}
`
)

func shellOut(cmd string) {
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to execute %v: %v, err: %v", cmd, string(out), err)
	}
}

func restartHaproxy(cmd string) {

	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		log.Printf("Failed to parse haproxy.cfg %v: %v, err: %v", cmd, string(out), err)
		log.Printf("Not restarting haproxy.")
	} else {
		log.Printf("Haproxy.cfg looks good.  Restart haproxy")
		//to reload a new cfgiguration with minimal service impact and without
		//breaking existing sessions :
		shellOut("haproxy -f /etc/haproxy/haproxy.cfg -p /var/run/haproxy-private.pid -st /var/run/haproxy-private.pid")
	}
}

func main() {
	var ingClient client.IngressInterface
	if kubeClient, err := client.NewInCluster(); err != nil {
		log.Fatalf("Failed to create client: %v.", err)
	} else {
		ingClient = kubeClient.Extensions().Ingress(os.Getenv("INGRESS_NAMESPACE"))
	}
	tmpl, _ := template.New("haproxy").Parse(haproxyConf)
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
		if w, err := os.Create("/etc/haproxy/haproxy.cfg"); err != nil {
			log.Fatalf("Failed to open %v: %v", haproxyConf, err)
		} else if err := tmpl.Execute(w, ingresses); err != nil {
			log.Fatalf("Failed to write template %v", err)
		}

		restartHaproxy("haproxy -t")

	}
}

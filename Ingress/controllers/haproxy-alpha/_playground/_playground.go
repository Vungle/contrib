package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/template"
)

const (
	haproxyConf = `
global
    daemon
    stats socket /tmp/haproxy
		nbproc  2
		pidfile /var/run/haproxy-private.pid

defaults
    log global
    option  httplog


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

# haproxy stats, required hostport and firewall rules for :1936
listen stats
    bind *:1936
    stats enable
    stats hide-version
    stats realm Haproxy\ Statistics
    stats uri /

frontend http
    # Frontend bound on all network interfaces on port 80
    bind *:80

{{range $ing := .Items}}
{{range $rule := $ing.Spec.Rules}}
	{{ range $path := $rule.HTTP.Paths }}
	acl host_acl_{{$path.Backend.ServiceName}} hdr(host) {{$rule.Host}}
	acl url_acl_{{$rule.Host}} path_beg {{$path.Path}}
	use_backend {{$path.Backend.ServiceName}} if url_acl_{{$rule.Host}} host_acl_{{$path.Backend.ServiceName}}
	{{end}}
{{end}}
{{end}}

{{range $ing := .Items}}
{{range $rule := $ing.Spec.Rules}}
{{ range $path := $rule.HTTP.Paths }}
backend {{$path.Backend.ServiceName}}
	server {{$path.Backend.ServiceName}} {{$path.Backend.ServiceName}}.{{$ing.Metadata.Namespace}}.svc.cluster.local:{{$path.Backend.ServicePort}} check
  {{end}}

{{end}}
{{end}}
`
)

type Paths []struct {
	Path    string `json:"path"`
	Backend struct {
		ServiceName string `json:"serviceName"`
		ServicePort int    `json:"servicePort"`
	} `json:"backend"`
}

func (p Paths) Len() int {
	return len(p)
}

func (p Paths) Less(i, j int) bool {
	return p[i].Path < p[j].Path
}

func (p Paths) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

type Ingress struct {
	Kind       string   `json:"kind"`
	APIVersion string   `json:"apiVersion"`
	Metadata   struct{} `json:"metadata"`
	Items      []struct {
		Kind       string `json:"kind"`
		APIVersion string `json:"apiVersion"`
		Metadata   struct {
			Name            string `json:"name"`
			Namespace       string `json:"namespace"`
			SelfLink        string `json:"selfLink"`
			UID             string `json:"uid"`
			ResourceVersion string `json:"resourceVersion"`
			Generation      int    `json:"generation"`
		} `json:"metadata"`
		Spec struct {
			Rules []struct {
				Host string `json:"host"`
				HTTP struct {
					Paths []struct {
						Path    string `json:"path"`
						Backend struct {
							ServiceName string `json:"serviceName"`
							ServicePort int    `json:"servicePort"`
						} `json:"backend"`
					} `json:"paths"`
				} `json:"http"`
			} `json:"rules"`
		} `json:"spec"`
		Status struct {
			LoadBalancer struct {
			} `json:"loadBalancer"`
		} `json:"status"`
	} `json:"items"`
}

func main() {
	fmt.Println("Hello, playground")
	tmpl, _ := template.New("haproxy").Parse(haproxyConf)
	response := []byte(`{"kind":"List","apiVersion":"v1","metadata":{},"items":[{"kind":"Ingress","apiVersion":"extensions/v1beta1","metadata":{"name":"aa-server-port-80","namespace":"default","selfLink":"/apis/extensions/v1beta1/namespaces/default/ingresses/aa-server-port-80","uid":"7defad5e-db41-11e5-9156-12cccd293299","resourceVersion":"133729342","generation":3,"creationTimestamp":"2016-02-24T21:56:43Z"},"spec":{"rules":[{"host":"server-port-default.kube-prod1.vungle.io","http":{"paths":[{"path":"/","backend":{"serviceName":"aa-server-port-80-svc","servicePort":80}},{"path":"/.well-known/acme-challenge","backend":{"serviceName":"lets-encrypt","servicePort":80}}]}}]},"status":{"loadBalancer":{}}},{"kind":"Ingress","apiVersion":"extensions/v1beta1","metadata":{"name":"billboard","namespace":"default","selfLink":"/apis/extensions/v1beta1/namespaces/default/ingresses/billboard","uid":"900dc71a-cb8b-11e5-b8f7-12cccd293299","resourceVersion":"133703561","generation":2,"creationTimestamp":"2016-02-04T22:06:38Z"},"spec":{"rules":[{"host":"billboard.vungle.com","http":{"paths":[{"path":"/","backend":{"serviceName":"billboard","servicePort":80}},{"path":"/.well-known/acme-challenge","backend":{"serviceName":"lets-encrypt","servicePort":80}}]}}]},"status":{"loadBalancer":{}}},{"kind":"Ingress","apiVersion":"extensions/v1beta1","metadata":{"name":"data-api-temp-external","namespace":"default","selfLink":"/apis/extensions/v1beta1/namespaces/default/ingresses/data-api-temp-external","uid":"74b4106d-eaef-11e5-a2af-1213d0960275","resourceVersion":"173827720","generation":2,"creationTimestamp":"2016-03-15T20:49:48Z"},"spec":{"rules":[{"host":"data-ext.vungle.com","http":{"paths":[{"path":"/","backend":{"serviceName":"data-api-temp-external","servicePort":80}},{"path":"/.well-known/acme-challenge","backend":{"serviceName":"lets-encrypt","servicePort":80}}]}}]},"status":{"loadBalancer":{}}},{"kind":"Ingress","apiVersion":"extensions/v1beta1","metadata":{"name":"dataingestion","namespace":"default","selfLink":"/apis/extensions/v1beta1/namespaces/default/ingresses/dataingestion","uid":"521f5616-f533-11e5-8cde-1213d0960275","resourceVersion":"213651089","generation":1,"creationTimestamp":"2016-03-28T22:20:47Z"},"spec":{"rules":[{"host":"ingest.vungle.com","http":{"paths":[{"path":"/eventData","backend":{"serviceName":"dataingestioneventdata","servicePort":80}},{"path":"/api/v1/sdkErrors","backend":{"serviceName":"dataingestionsdk","servicePort":80}},{"path":"/tpat","backend":{"serviceName":"dataingestiontpat","servicePort":80}}]}}]},"status":{"loadBalancer":{}}},{"kind":"Ingress","apiVersion":"extensions/v1beta1","metadata":{"name":"docker-registry","namespace":"default","selfLink":"/apis/extensions/v1beta1/namespaces/default/ingresses/docker-registry","uid":"43df621c-db34-11e5-9156-12cccd293299","resourceVersion":"130787889","generation":1,"creationTimestamp":"2016-02-24T20:22:02Z"},"spec":{"rules":[{"host":"vungle.io","http":{"paths":[{"path":"/","backend":{"serviceName":"docker-registry","servicePort":5000}},{"path":"/.well-known/acme-challenge","backend":{"serviceName":"lets-encrypt","servicePort":80}}]}}]},"status":{"loadBalancer":{}}},{"kind":"Ingress","apiVersion":"extensions/v1beta1","metadata":{"name":"elasticsearch","namespace":"default","selfLink":"/apis/extensions/v1beta1/namespaces/default/ingresses/elasticsearch","uid":"6c37887a-da56-11e5-9156-12cccd293299","resourceVersion":"173837944","generation":2,"creationTimestamp":"2016-02-23T17:54:02Z"},"spec":{"rules":[{"host":"elastikube.vungle.com","http":{"paths":[{"path":"/","backend":{"serviceName":"elasticsearch","servicePort":80}},{"path":"/.well-known/acme-challenge","backend":{"serviceName":"lets-encrypt","servicePort":80}}]}}]},"status":{"loadBalancer":{}}},{"kind":"Ingress","apiVersion":"extensions/v1beta1","metadata":{"name":"gor-replay","namespace":"default","selfLink":"/apis/extensions/v1beta1/namespaces/default/ingresses/gor-replay","uid":"35bfc8e7-da74-11e5-9156-12cccd293299","resourceVersion":"173839383","generation":2,"creationTimestamp":"2016-02-23T21:27:15Z"},"spec":{"rules":[{"host":"gor-replay.vungle.io","http":{"paths":[{"path":"/","backend":{"serviceName":"gor-replay","servicePort":80}},{"path":"/.well-known/acme-challenge","backend":{"serviceName":"lets-encrypt","servicePort":80}}]}}]},"status":{"loadBalancer":{}}},{"kind":"Ingress","apiVersion":"extensions/v1beta1","metadata":{"name":"grafana","namespace":"default","selfLink":"/apis/extensions/v1beta1/namespaces/default/ingresses/grafana","uid":"b38792da-db24-11e5-9156-12cccd293299","resourceVersion":"173840297","generation":2,"creationTimestamp":"2016-02-24T18:30:38Z"},"spec":{"rules":[{"host":"shh.vungle.io","http":{"paths":[{"path":"/","backend":{"serviceName":"influx-udp","servicePort":80}},{"path":"/.well-known/acme-challenge","backend":{"serviceName":"lets-encrypt","servicePort":80}}]}}]},"status":{"loadBalancer":{}}},{"kind":"Ingress","apiVersion":"extensions/v1beta1","metadata":{"name":"influx-udp","namespace":"default","selfLink":"/apis/extensions/v1beta1/namespaces/default/ingresses/influx-udp","uid":"180be6d1-e0cd-11e5-9156-12cccd293299","resourceVersion":"141207774","generation":1,"creationTimestamp":"2016-03-02T23:18:38Z"},"spec":{"rules":[{"host":"influxdb-udp.vungle.io","http":{"paths":[{"path":"/","backend":{"serviceName":"influx-udp","servicePort":80}}]}}]},"status":{"loadBalancer":{}}},{"kind":"Ingress","apiVersion":"extensions/v1beta1","metadata":{"name":"jaeger","namespace":"default","selfLink":"/apis/extensions/v1beta1/namespaces/default/ingresses/jaeger","uid":"24e29abb-f53b-11e5-8cde-1213d0960275","resourceVersion":"213800874","generation":1,"creationTimestamp":"2016-03-28T23:16:47Z"},"spec":{"rules":[{"host":"api.vungle.com","http":{"paths":[{"path":"/","backend":{"serviceName":"jaeger","servicePort":80}},{"path":"/.well-known/acme-challenge","backend":{"serviceName":"lets-encrypt","servicePort":80}}]}}]},"status":{"loadBalancer":{}}},{"kind":"Ingress","apiVersion":"extensions/v1beta1","metadata":{"name":"kubana","namespace":"default","selfLink":"/apis/extensions/v1beta1/namespaces/default/ingresses/kubana","uid":"5073116e-d4f6-11e5-9156-12cccd293299","resourceVersion":"133695558","generation":2,"creationTimestamp":"2016-02-16T21:43:28Z"},"spec":{"rules":[{"host":"kubana.vungle.com","http":{"paths":[{"path":"/","backend":{"serviceName":"kibana","servicePort":80}},{"path":"/.well-known/acme-challenge","backend":{"serviceName":"lets-encrypt","servicePort":80}}]}}]},"status":{"loadBalancer":{}}},{"kind":"Ingress","apiVersion":"extensions/v1beta1","metadata":{"name":"ltv-data-api","namespace":"default","selfLink":"/apis/extensions/v1beta1/namespaces/default/ingresses/ltv-data-api","uid":"13ca6264-d999-11e5-9156-12cccd293299","resourceVersion":"211674600","generation":6,"creationTimestamp":"2016-02-22T19:18:38Z"},"spec":{"rules":[{"host":"ltv-data-api.kube-prod.vungle.com","http":{"paths":[{"path":"/","backend":{"serviceName":"ltv-data-api","servicePort":9000}}]}},{"host":"pie.api.vungle.com","http":{"paths":[{"path":"/","backend":{"serviceName":"ltv-data-api","servicePort":9000}}]}},{"host":"data.vungle.com","http":{"paths":[{"path":"/pie","backend":{"serviceName":"ltv-data-api","servicePort":9000}}]}}]},"status":{"loadBalancer":{}}},{"kind":"Ingress","apiVersion":"extensions/v1beta1","metadata":{"name":"viking-api","namespace":"default","selfLink":"/apis/extensions/v1beta1/namespaces/default/ingresses/viking-api","uid":"7433c964-f065-11e5-8cde-1213d0960275","resourceVersion":"193807800","generation":1,"creationTimestamp":"2016-03-22T19:37:03Z"},"spec":{"rules":[{"host":"viking.vungle.com","http":{"paths":[{"path":"/","backend":{"serviceName":"viking-api","servicePort":80}},{"path":"/.well-known/acme-challenge","backend":{"serviceName":"lets-encrypt","servicePort":80}}]}}]},"status":{"loadBalancer":{}}}]}
`)
	var ingresses Ingress
	json.Unmarshal(response, &ingresses)
	for _, items := range ingresses.Items {
		for _, rule := range items.Spec.Rules {
			ps := make(Paths, 0, len(rule.HTTP.Paths))

			for _, path := range rule.HTTP.Paths {
				ps = append(ps, path)
				//fmt.Println(paths.Path)
			}

			sort.Sort(sort.Reverse(ps))

			for i, _ := range ps {
				rule.HTTP.Paths[i] = ps[i]
			}
		}
	}
	tmpl.Execute(os.Stdout, ingresses)
}

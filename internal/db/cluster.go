package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	k8sclient "github.com/your/module/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

// ConnectForK8s discovers RethinkDB service address using the provided per-cluster k8s client.
// addrOverride takes precedence when non-empty. user/pass are optional.
func ConnectForK8s(ctx context.Context, kc *k8sclient.Client, addrOverride, user, pass string) (*Manager, error) {
	addr := strings.TrimSpace(addrOverride)
	if addr == "" {
		svcName := strings.TrimSpace(os.Getenv("RETHINKDB_SERVICE_NAME"))
		if svcName == "" {
			svcName = "rethinkdb"
		}
		ns := strings.TrimSpace(os.Getenv("RETHINKDB_NAMESPACE"))
		if ns == "" {
			ns = "default"
		}
		// Try LoadBalancer/NodePort/ClusterIP in that order
		if svc, err := kc.K.CoreV1().Services(ns).Get(ctx, svcName, metav1.GetOptions{}); err == nil && svc != nil {
			// LB
			if ing := svc.Status.LoadBalancer.Ingress; len(ing) > 0 {
				host := ing[0].IP
				if host == "" {
					host = ing[0].Hostname
				}
				port := int32(28015)
				for _, sp := range svc.Spec.Ports {
					if sp.Name == "client" || sp.Port == 28015 {
						port = sp.Port
						break
					}
				}
				if host != "" && port > 0 {
					addr = fmt.Sprintf("%s:%d", host, port)
				}
			}
			// NodePort
			if addr == "" && svc.Spec.Type == corev1.ServiceTypeNodePort {
				var nodePort int32
				for _, sp := range svc.Spec.Ports {
					if sp.Name == "client" || sp.Port == 28015 {
						nodePort = sp.NodePort
						break
					}
				}
				if nodePort == 0 && len(svc.Spec.Ports) > 0 {
					nodePort = svc.Spec.Ports[0].NodePort
				}
				if nodePort > 0 {
					if nodes, err := kc.K.CoreV1().Nodes().List(ctx, metav1.ListOptions{}); err == nil {
						for _, n := range nodes.Items {
							for _, a := range n.Status.Addresses {
								if a.Type == corev1.NodeExternalIP && strings.TrimSpace(a.Address) != "" {
									addr = fmt.Sprintf("%s:%d", a.Address, nodePort)
									break
								}
								if a.Type == corev1.NodeInternalIP && strings.TrimSpace(a.Address) != "" {
									addr = fmt.Sprintf("%s:%d", a.Address, nodePort)
									break
								}
							}
							if addr != "" {
								break
							}
						}
					}
				}
			}
			// ClusterIP
			if addr == "" && svc.Spec.ClusterIP != "" && svc.Spec.ClusterIP != "None" {
				port := int32(28015)
				for _, sp := range svc.Spec.Ports {
					if sp.Name == "client" || sp.Port == 28015 {
						port = sp.Port
						break
					}
				}
				addr = fmt.Sprintf("%s:%d", svc.Spec.ClusterIP, port)
			}
		}
	}
	if addr == "" {
		addr = "127.0.0.1:28015"
	}
	opts := r.ConnectOpts{Address: addr, InitialCap: 2, MaxOpen: 10, Timeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second}
	if strings.TrimSpace(user) != "" {
		opts.Username = strings.TrimSpace(user)
	}
	if strings.TrimSpace(pass) != "" {
		opts.Password = strings.TrimSpace(pass)
	}
	sess, err := r.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("rethinkdb connect failed addr=%s: %w", addr, err)
	}
	return &Manager{sess: sess}, nil
}

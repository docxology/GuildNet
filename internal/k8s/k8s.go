package k8s

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/your/module/internal/model"
)

type Client struct {
	K *kubernetes.Clientset
}

func kubeconfigDefault() string {
	if v := os.Getenv("KUBECONFIG"); v != "" {
		return v
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".kube", "config")
	}
	return ""
}

func New(ctx context.Context) (*Client, error) {
	var cfg *rest.Config
	var err error
	cfg, err = rest.InClusterConfig()
	if err != nil {
		// fallback to kubeconfig
		kc := kubeconfigDefault()
		if kc == "" {
			return nil, fmt.Errorf("no in-cluster config and no kubeconfig")
		}
		cfg, err = clientcmd.BuildConfigFromFlags("", kc)
		if err != nil {
			return nil, err
		}
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{K: cs}, nil
}

func dns1123Name(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			prevDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == '-' || r == '_' || r == ' ':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	res := strings.Trim(b.String(), "-")
	for strings.Contains(res, "--") {
		res = strings.ReplaceAll(res, "--", "-")
	}
	if res == "" {
		res = "workload"
	}
	return res
}

type EnsureOpts struct {
	Namespace string
	ID        string // stable id label
}

// EnsureDeploymentAndService creates or updates a Deployment and Service for the job spec.
func (c *Client) EnsureDeploymentAndService(ctx context.Context, spec model.JobSpec, opt EnsureOpts) (name string, id string, err error) {
	ns := opt.Namespace
	if ns == "" {
		ns = "default"
	}
	name = dns1123Name(spec.Name)
	if name == "" {
		name = dns1123Name(spec.Image)
	}
	if opt.ID == "" {
		id = name
	} else {
		id = opt.ID
	}

	labels := map[string]string{
		"app":                    name,
		"app.kubernetes.io/name": name,
		"guildnet.io/managed":    "true",
		"guildnet.io/id":         id,
	}

	// container ports
	cports := []corev1.ContainerPort{}
	for _, p := range spec.Expose {
		if p.Port <= 0 {
			continue
		}
		cp := corev1.ContainerPort{ContainerPort: int32(p.Port)}
		if p.Name != "" {
			cp.Name = p.Name
		}
		cports = append(cports, cp)
	}

	// env
	env := []corev1.EnvVar{}
	for k, v := range spec.Env {
		env = append(env, corev1.EnvVar{Name: k, Value: v})
	}
	sort.Slice(env, func(i, j int) bool { return env[i].Name < env[j].Name })

	// Deployment
	replicas := int32(1)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:           "app",
					Image:          spec.Image,
					Args:           spec.Args,
					Env:            env,
					Ports:          cports,
					ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: intstr.FromInt(8080)}}},
					LivenessProbe:  &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: intstr.FromInt(8080)}}},
				}}},
			},
		},
	}

	// Create or update
	if _, err := c.K.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{}); err == nil {
		if _, err := c.K.AppsV1().Deployments(ns).Update(ctx, dep, metav1.UpdateOptions{}); err != nil {
			return "", "", err
		}
	} else {
		if _, err := c.K.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{}); err != nil {
			return "", "", err
		}
	}

	// Service ports
	sports := []corev1.ServicePort{}
	if len(spec.Expose) == 0 && len(cports) == 0 {
		sports = append(sports, corev1.ServicePort{Name: "http", Port: 8080, TargetPort: intstr.FromInt(8080)})
	} else {
		for _, p := range spec.Expose {
			if p.Port <= 0 {
				continue
			}
			nm := p.Name
			if nm == "" {
				nm = fmt.Sprintf("p-%d", p.Port)
			}
			sports = append(sports, corev1.ServicePort{Name: nm, Port: int32(p.Port), TargetPort: intstr.FromInt(p.Port)})
		}
		if len(sports) == 0 {
			for _, cp := range cports {
				sports = append(sports, corev1.ServicePort{Name: cp.Name, Port: cp.ContainerPort, TargetPort: intstr.FromInt(int(cp.ContainerPort))})
			}
		}
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": name}, Ports: sports},
	}
	if _, err := c.K.CoreV1().Services(ns).Get(ctx, name, metav1.GetOptions{}); err == nil {
		if _, err := c.K.CoreV1().Services(ns).Update(ctx, svc, metav1.UpdateOptions{}); err != nil {
			return "", "", err
		}
	} else {
		if _, err := c.K.CoreV1().Services(ns).Create(ctx, svc, metav1.CreateOptions{}); err != nil {
			return "", "", err
		}
	}
	return name, id, nil
}

// ListServers returns Deployments managed by GuildNet mapped into model.Server.
func (c *Client) ListServers(ctx context.Context, ns string) ([]*model.Server, error) {
	if ns == "" {
		ns = "default"
	}
	depList, err := c.K.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{LabelSelector: "guildnet.io/managed=true"})
	if err != nil {
		return nil, err
	}
	out := []*model.Server{}
	for _, d := range depList.Items {
		id := d.Labels["guildnet.io/id"]
		if id == "" {
			id = d.Name
		}
		status := "pending"
		if d.Status.ReadyReplicas > 0 {
			status = "running"
		}
		ports := []model.Port{}
		// try service ports
		svc, _ := c.K.CoreV1().Services(ns).Get(ctx, d.Name, metav1.GetOptions{})
		if svc != nil {
			for _, sp := range svc.Spec.Ports {
				ports = append(ports, model.Port{Name: sp.Name, Port: int(sp.Port)})
			}
		}
		env := map[string]string{}
		if len(d.Spec.Template.Spec.Containers) > 0 {
			for _, e := range d.Spec.Template.Spec.Containers[0].Env {
				env[e.Name] = e.Value
			}
		}
		out = append(out, &model.Server{
			ID:     id,
			Name:   d.Name,
			Image:  firstImage(d),
			Status: status,
			Ports:  ports,
			Env:    env,
		})
	}
	return out, nil
}

func firstImage(d appsv1.Deployment) string {
	if len(d.Spec.Template.Spec.Containers) > 0 {
		return d.Spec.Template.Spec.Containers[0].Image
	}
	return ""
}

// ResolveServiceAddress returns host and port candidates for a given id/name.
func (c *Client) ResolveServiceAddress(ctx context.Context, ns, idOrName string) (host string, port int, https bool, err error) {
	if ns == "" {
		ns = "default"
	}
	// prefer by name; fallback by label selection
	svc, err1 := c.K.CoreV1().Services(ns).Get(ctx, idOrName, metav1.GetOptions{})
	if err1 != nil {
		// try by label selector guildnet.io/id
		list, err2 := c.K.CoreV1().Services(ns).List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("guildnet.io/id=%s", idOrName)})
		if err2 != nil || len(list.Items) == 0 {
			return "", 0, false, fmt.Errorf("service not found for %s", idOrName)
		}
		svc = &list.Items[0]
	}
	if svc.Spec.ClusterIP == "" || svc.Spec.ClusterIP == "None" {
		return "", 0, false, fmt.Errorf("service has no clusterIP")
	}
	// choose port: prefer 8443/https else 8080 else first
	var p int32
	https = false
	for _, sp := range svc.Spec.Ports {
		if sp.Port == 8443 {
			p = sp.Port
			https = true
			break
		}
	}
	if p == 0 {
		for _, sp := range svc.Spec.Ports {
			if sp.Port == 8080 {
				p = sp.Port
				break
			}
		}
	}
	if p == 0 && len(svc.Spec.Ports) > 0 {
		p = svc.Spec.Ports[0].Port
	}
	if p == 0 {
		return "", 0, false, fmt.Errorf("service has no ports")
	}
	return svc.Spec.ClusterIP, int(p), https, nil
}

// GetServer returns a single server by id or name.
func (c *Client) GetServer(ctx context.Context, ns, idOrName string) (*model.Server, error) {
	list, err := c.ListServers(ctx, ns)
	if err != nil {
		return nil, err
	}
	for _, s := range list {
		if s.ID == idOrName || s.Name == idOrName {
			return s, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

// GetLogs tails recent container logs from the first pod of the server's Deployment.
func (c *Client) GetLogs(ctx context.Context, ns, idOrName, level string, limit int) ([]model.LogLine, error) {
	if ns == "" {
		ns = "default"
	}
	// find deployment by name or label
	var dep *appsv1.Deployment
	if d, err := c.K.AppsV1().Deployments(ns).Get(ctx, idOrName, metav1.GetOptions{}); err == nil {
		dep = d
	} else {
		lst, err2 := c.K.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("guildnet.io/id=%s", idOrName)})
		if err2 != nil || len(lst.Items) == 0 {
			return nil, fmt.Errorf("deployment not found")
		}
		dd := lst.Items[0]
		dep = &dd
	}
	if dep == nil {
		return nil, fmt.Errorf("deployment not found")
	}
	// list pods
	pods, err := c.K.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", dep.Name)})
	if err != nil || len(pods.Items) == 0 {
		return nil, fmt.Errorf("no pods")
	}
	pod := pods.Items[0]
	container := ""
	if len(pod.Spec.Containers) > 0 {
		container = pod.Spec.Containers[0].Name
	}
	// fetch logs
	tail := int64(limit)
	req := c.K.CoreV1().Pods(ns).GetLogs(pod.Name, &corev1.PodLogOptions{Container: container, TailLines: &tail})
	data, err := req.Do(ctx).Raw()
	if err != nil {
		return nil, err
	}
	// split by lines
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	out := make([]model.LogLine, 0, len(lines))
	for _, ln := range lines {
		out = append(out, model.LogLine{T: model.NowISO(), LVL: level, MSG: ln})
	}
	return out, nil
}

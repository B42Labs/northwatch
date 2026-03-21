package enrich

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type PodInfo struct {
	Name      string
	Namespace string
	Labels    map[string]string
	NodeName  string
}

type ServiceInfo struct {
	Name      string
	Namespace string
	ClusterIP string
}

// KubernetesProvider enriches OVN entities with Kubernetes metadata.
// Function fields allow testing without a real Kubernetes API.
type KubernetesProvider struct {
	LookupPod     func(ctx context.Context, namespace, name string) (*PodInfo, error)
	LookupService func(ctx context.Context, namespace, name string) (*ServiceInfo, error)
}

// NewKubernetesProvider creates a KubernetesProvider with a real Kubernetes client.
// If kubeconfig is empty, in-cluster config is attempted.
// kubeContext overrides the current-context in the kubeconfig if non-empty.
func NewKubernetesProvider(_ context.Context, kubeconfig, kubeContext string) (*KubernetesProvider, error) {
	var cfg *rest.Config
	var err error

	if kubeconfig != "" {
		rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
		overrides := &clientcmd.ConfigOverrides{}
		if kubeContext != "" {
			overrides.CurrentContext = kubeContext
		}
		cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	} else {
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("building Kubernetes config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating Kubernetes clientset: %w", err)
	}

	return &KubernetesProvider{
		LookupPod: func(ctx context.Context, namespace, name string) (*PodInfo, error) {
			pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			return &PodInfo{
				Name:      pod.Name,
				Namespace: pod.Namespace,
				Labels:    pod.Labels,
				NodeName:  pod.Spec.NodeName,
			}, nil
		},
		LookupService: func(ctx context.Context, namespace, name string) (*ServiceInfo, error) {
			svc, err := clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			return &ServiceInfo{
				Name:      svc.Name,
				Namespace: svc.Namespace,
				ClusterIP: svc.Spec.ClusterIP,
			}, nil
		},
	}, nil
}

func (*KubernetesProvider) Name() string { return "kubernetes" }

func (p *KubernetesProvider) EnrichPort(ctx context.Context, externalIDs map[string]string) (*Info, error) {
	podRef := externalIDs["k8s.ovn.org/pod"]
	if podRef == "" {
		return nil, nil
	}

	namespace, name, ok := parseNamespacedName(podRef)
	if !ok {
		return nil, nil
	}

	info := &Info{
		DisplayName: name,
		ProjectName: namespace,
		Extra:       make(map[string]string),
	}

	if nad := externalIDs["k8s.ovn.org/nad"]; nad != "" {
		info.Extra["nad"] = nad
	}

	if p.LookupPod != nil {
		pod, err := p.LookupPod(ctx, namespace, name)
		if err == nil {
			info.Extra["node"] = pod.NodeName
			for k, v := range pod.Labels {
				info.Extra["label:"+k] = v
			}
		}
	}

	return info, nil
}

func (p *KubernetesProvider) EnrichNetwork(_ context.Context, externalIDs map[string]string) (*Info, error) {
	network := externalIDs["k8s.ovn.org/network"]
	if network == "" {
		return nil, nil
	}
	return &Info{DisplayName: network}, nil
}

func (p *KubernetesProvider) EnrichRouter(_ context.Context, externalIDs map[string]string) (*Info, error) {
	network := externalIDs["k8s.ovn.org/network"]
	if network == "" {
		return nil, nil
	}
	return &Info{DisplayName: network}, nil
}

func (p *KubernetesProvider) EnrichNAT(_ context.Context, _ map[string]string) (*Info, error) {
	return nil, nil
}

func parseNamespacedName(ref string) (namespace, name string, ok bool) {
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

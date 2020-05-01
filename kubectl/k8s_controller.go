package kubectl

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// k8s Controller.
// |deploymentList|, |nodeList| and |podList| are variables for storing k8s information.
// |namespace| is the namespace in kubernetes that the system will use.
// |client| is API used for finding resources.
// |dynamicClient| is API for managing deployments.
type KubeController struct {
	namespace      string
	client         *kubernetes.Clientset
	dynamicClient  dynamic.Interface
	deploymentList *sync.Map
	nodeList       atomic.Value
	podList        *sync.Map
}

var K8sHandler *KubeController

// Initialize global instance for KubeController
// init() is guaranteed to run before main() is called.
func init() {
	// Loads the kube-config rules.
	var kubeConfig *string

	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	if home != "" {
		kubeConfig = flag.String("config",
			filepath.Join(home, ".kube", "config"),
			"(optional) absolute path to the kubeconfig file")
	} else {
		kubeConfig = flag.String("config",
			"",
			"absolute path to the kubeconfig file")
	}

	var err error
	K8sHandler, err = NewKubeController(*kubeConfig, "openfaas-fn")
	if err != nil || K8sHandler == nil {
		fmt.Printf("Failed to connect to the Kubernetes cluster. Exit..")
		os.Exit(-1)
	}
}

// Creates a new k8s Controller object.
func NewKubeController(kubeConfig string, namespace string) (*KubeController, error) {
	fmt.Printf("Creates a k8s controller.\n")

	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &KubeController{
		namespace:      namespace,
		client:         client,
		dynamicClient:  dynamicClient,
		podList:        new(sync.Map),
		deploymentList: new(sync.Map),
	}, nil
}

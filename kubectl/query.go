package kubectl

import (
	"fmt"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/labels"
)

func (k8s *KubeController) FetchPods() {
	l, _ := k8s.client.CoreV1().Pods(k8s.namespace).List(metav1.ListOptions{})
	k8s.podList.Store(k8s.namespace, l)
}

func (k8s *KubeController) PrintPods() {
	podCache, isSuccess := k8s.podList.Load(k8s.namespace)
	if !isSuccess {
		return
	}

	l, isSuccess := podCache.(*corev1.PodList)
	if !isSuccess {
		return
	}

	fmt.Printf("List all pods.\n")
	fmt.Printf("| %-35s| %-8s| %-18s|\n", "Pod", "Node", "Status")

	for i := range l.Items {
		status := ""
		if l.Items[i].DeletionTimestamp != nil {
			status = "Terminating"
		} else {
			if len(l.Items[i].Status.ContainerStatuses) == 0 {
				status = "Error"
			} else {
				containerState := l.Items[i].Status.ContainerStatuses[0].State
				if containerState.Running != nil {
					status = "Running"
				} else if containerState.Waiting != nil {
					status = containerState.Waiting.Reason
				} else if containerState.Terminated != nil {
					status = "Terminating"
				}
			}
		}
		fmt.Printf("| %-35s| %-8s| %-18s|\n", l.Items[i].Name, l.Items[i].Spec.NodeName, status)
	}
}

// Get a pod with its label "nodeName-funcType-hostPort" from fetched results.
// Note: Remember to call function FetchPods before.
func (k8s *KubeController) GetPodByLabel(nodeName string, funcType string, hostPort int) (corev1.Pod, bool) {
	podCache, isSuccess := k8s.podList.Load(k8s.namespace)
	if !isSuccess {
		return corev1.Pod{}, false
	}

	l, isSuccess := podCache.(*corev1.PodList)
	if !isSuccess {
		return corev1.Pod{}, false
	}

	label := fmt.Sprintf("%s-%s-%s", nodeName, funcType, strconv.Itoa(hostPort))
	for i := range l.Items {
		if l.Items[i].Labels != nil && l.Items[i].Labels["app"] == label {
			return l.Items[i], true
		}
	}
	return corev1.Pod{}, false
}

// Checks and returns the pod's status with its label "deployName".
// Note: No need to call function FetchPods before.
func (k8s *KubeController) GetPodStatusByName(deployName string) string {
	labelsMapping := map[string]string{"app": deployName}
	set := labels.Set(labelsMapping)
	// pod, _ := clientset.CoreV1().Pods(k8s.namespace).Get(pod.Name, metav1.GetOptions{LabelSelector: set.AsSelector().String()})
	pods, _ := k8s.client.CoreV1().Pods(k8s.namespace).List(metav1.ListOptions{LabelSelector: set.AsSelector().String()})

	status := "NotExist"
	for _, pod := range pods.Items {
		if pod.DeletionTimestamp != nil {
			status = "Terminating"
		} else if len(pod.Status.ContainerStatuses) > 0 {
			if pod.Status.ContainerStatuses[0].State.Running != nil {
				status = "Running"
			} else if pod.Status.ContainerStatuses[0].State.Terminated != nil {
				status = "Terminating"
			}
		}
	}

	return status
}

func (k8s *KubeController) FetchDeployments() {
	l, _ := k8s.client.AppsV1().Deployments(k8s.namespace).List(metav1.ListOptions{})
	k8s.deploymentList.Store(k8s.namespace, l)
}

func formatDuration(duration time.Duration) string {
	if duration.Hours() > 1 {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	} else if duration.Minutes() > 1 {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	} else {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	}
}

func (k8s *KubeController) PrintDeployments() {
	deploymentCache, isSuccess := k8s.deploymentList.Load(k8s.namespace)
	if !isSuccess {
		return
	}

	l, isSuccess := deploymentCache.(*appsv1.DeploymentList)
	if !isSuccess {
		return
	}

	fmt.Printf("List all deployments.\n")
	fmt.Printf("| %-20s| %-6s| %-6s|\n", "Deployment", "Age", "Ready")
	for i := range l.Items {
		duration := time.Since(l.Items[i].CreationTimestamp.Time)
		fmt.Printf("| %-20s| %-6s| %-6s|\n", l.Items[i].Name, formatDuration(duration),
			fmt.Sprintf("%d/%d", l.Items[i].Status.ReadyReplicas, l.Items[i].Status.Replicas))
	}
}

// Get a deployment with name |deploymentName| from fetched results.
// Note: Remember to call function FetchDeployments before.
func (k8s *KubeController) GetDeploymentByName(deploymentName string) (appsv1.Deployment, bool) {
	return k8s.GetDeployment(deploymentName)
}

func (k8s *KubeController) GetDeployment(deploymentName string) (appsv1.Deployment, bool) {
	deploymentCache, isSuccess := k8s.deploymentList.Load(k8s.namespace)
	if !isSuccess {
		return appsv1.Deployment{}, false
	}

	l, isSuccess := deploymentCache.(*appsv1.DeploymentList)
	if !isSuccess {
		return appsv1.Deployment{}, false
	}

	for i := range l.Items {
		if deploymentName == l.Items[i].Name {
			return l.Items[i], true
		}
	}
	return appsv1.Deployment{}, false
}

func (k8s *KubeController) FetchNodes() {
	l, _ := k8s.client.CoreV1().Nodes().List(metav1.ListOptions{})
	k8s.nodeList.Store(l)
}

func (k8s *KubeController) PrintNodes() {
	l, isSuccess := k8s.nodeList.Load().(*corev1.NodeList)
	if !isSuccess {
		return
	}

	fmt.Printf("List all nodes.\n")
	fmt.Printf("| %-8s|\n", "Nodes")
	for i := range l.Items {
		fmt.Printf("| %-8s|\n", l.Items[i].Name)
	}
}

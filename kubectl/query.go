
package kubectl

import (
	"fmt"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (k8s *KubeController) GetPodByLabel(nfName string, instanceID int) (corev1.Pod, bool) {
	podCache, isSuccess := k8s.podList.Load(k8s.namespace)
	if !isSuccess {
		return corev1.Pod{}, false
	}

	l, isSuccess := podCache.(*corev1.PodList)
	if !isSuccess {
		return corev1.Pod{}, false
	}

	funcID := strconv.Itoa(instanceID)
	label := nfName + funcID
	// TODO: Modify the label for finding pod
	for i := range l.Items {
		if l.Items[i].Labels != nil && l.Items[i].Labels["app"] == label {
			return l.Items[i], true
		}
	}
	return corev1.Pod{}, false
}

// TODO: Remove unused function
//func (k8s *KubeController) QueryPodByLabel(nfName string, instanceID int) (*corev1.PodList, error) {
//	funcID := strconv.Itoa(instanceID)
//	labelsMapping := map[string] string {"app": nfName + funcID}
//	// TODO: Modify the label for finding pod
//	set := labels.Set(labelsMapping)
//
//	pods, err := k8s.client.CoreV1().Pods(k8s.namespace).List(metav1.ListOptions{LabelSelector: set.AsSelector().String()})
//	return pods, err
//}

func (k8s *KubeController) FetchDeployments() {
	l, _ := k8s.client.AppsV1().Deployments(k8s.namespace).List(metav1.ListOptions{})
	k8s.deploymentList.Store(k8s.namespace, l)
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
	fmt.Printf("| %-20s|\n", "Deployment")
	for i := range l.Items {
		fmt.Printf("| %-20s|\n", l.Items[i].Name)
	}
}

func (k8s *KubeController) GetDeploymentByFunctionIndex(nfName string, instanceID int) (appsv1.Deployment, bool) {
	deploymentName := toDPDKDeploymentName(nfName, instanceID)
	// TODO: Rename deployment
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

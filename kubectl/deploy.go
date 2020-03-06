
package kubectl

import (
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Defines all constants.
const kDockerhubUser string = "165749"
// All kinds of possible NFs.
var moduleNameMappings = map[string] string {
	"original": "None",
	"fc": "FlowCounter",
	"nat": "NAT",
	"filter": "Filter",
	"chacha": "CHACHA",
	"aesenc": "AESCBCEnc",
	"aesdec": "AESCBCDec",
}

// TODO: Rename deployment
func toDPDKDeploymentName(nfName string, funcIndex int) string {
	funcID := strconv.Itoa(funcIndex)
	return fmt.Sprintf("%s.dep.%s", nfName, funcID)
}

func (k8s *KubeController) generateDPDKDeployment(nfName string, funcIndex int,
	nodeName string, workerCore int, hostPort int) unstructured.Unstructured {
	coreID := strconv.Itoa(workerCore)
	funcID := strconv.Itoa(funcIndex)
	deploymentName := toDPDKDeploymentName(nfName, funcIndex)
	// TODO: Rename deployment
	moduleName, exists := moduleNameMappings[nfName]
	if !exists {
		moduleName  = "None"
	}

	deployment := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name": deploymentName,
			},
			"spec": map[string]interface{}{
				"replicas": 1,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app": nfName + funcID,
						// TODO: Modify label
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": nfName + funcID,
							// TODO: Modify label
						},
					},

					"spec": map[string]interface{}{
						"containers": []map[string]interface{}{
							{ // Container 0
								"name":  nfName,
								"image": kDockerhubUser + "/nf:base",
								"imagePullPolicy": "Always",
								"ports": []map[string]interface{}{
									{
										// The ports between [50052, 51051] on the host is used
										// for instance to receive gRPC requests.
										"containerPort": 50051,
										"hostPort": hostPort,
									},
								},
								"command": []string{
									"/app/main",
									"--vport=vport_"+coreID,
									"--worker_core="+coreID,
									"--module_name="+moduleName,
								},
								"volumeMounts": []map[string]interface{}{
									{ // volume 0
										"name": "pcidriver",
										"mountPath": "/sys/bus/pci/drivers",
									},
									{ // volume 1
										"name": "hugepage",
										"mountPath": "/sys/kernel/mm/hugepages",
									},
									{ // volume 2
										"name": "huge",
										"mountPath": "/mnt/huge",
									},
									{ // volume 3
										"name": "dev",
										"mountPath": "/dev",
									},
									{ // volume 4
										"name": "numa",
										"mountPath": "/sys/devices/system/node",
									},
									{ // volume 5
										"name": "runtime",
										"mountPath": "/var/run",
									},
									{ // volume 6
										"name": "port",
										"mountPath": "/tmp/sn_vports",
									},
								},
							},
						}, // Ends containers
						"nodeName": nodeName,
						"volumes": []map[string]interface{}{
							{ // volume 0
								"name": "pcidriver",
								"hostPath": map[string]interface{}{
									"path": "/sys/bus/pci/drivers",
								},
							},
							{ // volume 1
								"name": "hugepage",
								"hostPath": map[string]interface{}{
									"path": "/sys/kernel/mm/hugepages",
								},
							},
							{ // volume 2
								"name": "huge",
								"hostPath": map[string]interface{}{
									"path": "/mnt/huge",
								},
							},
							{ // volume 3
								"name": "dev",
								"hostPath": map[string]interface{}{
									"path": "/dev",
								},
							},
							{ // volume 4
								"name": "numa",
								"hostPath": map[string]interface{}{
									"path": "/sys/devices/system/node",
								},
							},
							{ // volume 5
								"name": "runtime",
								"hostPath": map[string]interface{}{
									"path": "/var/run",
								},
							},
							{ // volume 6
								"name": "port",
								"hostPath": map[string]interface{}{
									"path": "/tmp/sn_vports",
								},
							},
						}, // Ends volumes
					},
				},
			},
		},
	}

	return deployment
}

// Creates a DPDK FaaS Deployment for NF |nfName|, which will run at core |workerCore| of node |workerCore|.
func (k8s *KubeController) CreateDPDKDeployment(nfName string, funcIndex int,
	nodeName string, workerCore int, hostPort int) (*unstructured.Unstructured, error) {
	// TODO: Redesign the name of deployment.

	deploymentAPI := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	deploymentConfig := k8s.generateDPDKDeployment(nfName, funcIndex, nodeName, workerCore, hostPort)

	deployment, err := k8s.dynamicClient.Resource(deploymentAPI).Namespace(k8s.namespace).Create(&deploymentConfig, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("Failed to create Deployment[ns=%s,nf=%s] on Node[%s].\n", k8s.namespace, nfName, nodeName)
		return nil, err
	}

	return deployment, nil
}

// Deletes a FaaS Deployment by its |nfName| and |funcIndex|.
func (k8s *KubeController) DeleteDeployment(nfName string, funcIndex int) {
	// TODO: Redesign the name of deployment.
	deploymentIndex := strconv.Itoa(funcIndex)
	deploymentName := fmt.Sprintf("%s.dep.%s", nfName, deploymentIndex)

	deploymentAPI := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	deletePolicy := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}

	err := k8s.dynamicClient.Resource(deploymentAPI).Namespace(k8s.namespace).Delete(deploymentName, &deleteOptions)
	if err != nil {
		fmt.Printf("Failed to delete Deployment[ns=%s,nf=%s].\n", k8s.namespace, nfName)
		panic(err)
	}
}

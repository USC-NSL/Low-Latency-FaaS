package kubectl

import (
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Defines all constants.
const kDockerhubUser string = "ch8728847"

// All kinds of possible NFs.
var moduleNameMappings = map[string]string{
	"original": "None",
	"fc":       "FlowCounter",
	"nat":      "NAT",
	"filter":   "Filter",
	"chacha":   "CHACHA",
	"aesenc":   "AESCBCEnc",
	"aesdec":   "AESCBCDec",
	"acl":      "ACL",
}

// Create a NF instance with type |funcType| on node |nodeName|,
// also assign the port |hostPort| of the host for the instance to receive gRPC requests.
// In Kubernetes, the instance is run as a deployment with name "nodeName-funcType-portId".
func (k8s *KubeController) generateDPDKDeployment(nodeName string, funcType string, hostPort int,
	pcie string, isIngress string, isEgress string, vPortIncIdx int, vPortOutIdx int) unstructured.Unstructured {
	portId := strconv.Itoa(hostPort)
	vPortInc := strconv.Itoa(vPortIncIdx)
	vPortOut := strconv.Itoa(vPortOutIdx)

	deploymentName := fmt.Sprintf("%s-%s-%s", nodeName, funcType, portId)

	moduleName, exists := moduleNameMappings[funcType]
	if !exists {
		moduleName = "None"
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
						"app": deploymentName,
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": deploymentName,
						},
					},

					"spec": map[string]interface{}{
						"hostPID": true,
						"containers": []map[string]interface{}{
							{ // Container 0
								"securityContext": map[string]interface{}{
									"privileged": true,
									"runAsUser":  0,
								},
								"resources": map[string]interface{}{
									"limits": map[string]interface{}{
										"memory":        "1Gi",
										"hugepages-2Mi": "1Gi",
									},
								},
								"name":            funcType,
								"image":           kDockerhubUser + "/nf:latest",
								"imagePullPolicy": "Always",
								"ports": []map[string]interface{}{
									{
										// The ports between [50052, 51051] on the host is used
										// for instance to receive gRPC requests.
										"containerPort": 50051,
										"hostPort":      hostPort,
									},
								},
								"command": []string{
									//"sleep", "1500",
									"/app/main",
									"--node_name=" + nodeName,
									"--port=" + portId,
									"--module=" + moduleName,
									"--ingress=" + isIngress,
									"--egress=" + isEgress,
									"--isolation_key=" + pcie,
									"--device=" + pcie,
									"--vport_inc_idx=" + vPortInc,
									"--vport_out_idx=" + vPortOut,
								},
								"volumeMounts": []map[string]interface{}{
									{ // volume 0
										"name":      "pcidriver",
										"mountPath": "/sys/bus/pci/drivers",
										"readOnly":  false,
									},
									{ // volume 1
										"name":      "hugepage",
										"mountPath": "/sys/kernel/mm/hugepages",
										"readOnly":  false,
									},
									{ // volume 2
										"name":      "huge",
										"mountPath": "/mnt/huge",
										"readOnly":  false,
									},
									{ // volume 3
										"name":      "dev",
										"mountPath": "/dev",
										"readOnly":  false,
									},
									{ // volume 4
										"name":      "numa",
										"mountPath": "/sys/devices/system/node",
										"readOnly":  false,
									},
									{ // volume 5
										"name":      "runtime",
										"mountPath": "/var/run",
										"readOnly":  false,
									},
									{ // volume 6
										"name":      "port",
										"mountPath": "/tmp/sn_vports",
										"readOnly":  false,
									},
									{ // volume 7
										"name":      "pcidevice",
										"mountPath": "/sys/devices",
										"readOnly":  false,
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
							{ // volume 7
								"name": "pcidevice",
								"hostPath": map[string]interface{}{
									"path": "/sys/devices",
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

// Create a NF instance with type |funcType| on node |nodeName| at core |workerCore|,
// also assign the port |hostPort| of the host for the instance to receive gRPC requests.
// Essentially, it will call function generateDPDKDeployment to generate a deployment in kubernetes.
func (k8s *KubeController) CreateDeployment(nodeName string, funcType string, hostPort int,
	pcie string, isIngress string, isEgress string, vPortIncIdx int, vPortOutIdx int) (*unstructured.Unstructured, error) {
	deploymentAPI := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	deploymentConfig := k8s.generateDPDKDeployment(nodeName, funcType, hostPort, pcie, isIngress, isEgress, vPortIncIdx, vPortOutIdx)

	deployment, err := k8s.dynamicClient.Resource(deploymentAPI).Namespace(k8s.namespace).Create(&deploymentConfig, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	fmt.Printf("Create instance [%s] (pcie=%s,ingress=%s,egress=%s) on %s with port %d successfully.\n",
		funcType, pcie, isIngress, isEgress, nodeName, hostPort)
	return deployment, nil
}

// Delete a NF instance with type |funcType| on node |nodeName| at core |workerCore| with assigned port |hostPort|.
func (k8s *KubeController) DeleteDeployment(nodeName string, funcType string, hostPort int) error {
	deploymentName := fmt.Sprintf("%s-%s-%s", nodeName, funcType, strconv.Itoa(hostPort))
	return k8s.DeleteDeploymentByName(deploymentName)
}

// Delete a kubernetes deployment with the name |deploymentName|.
func (k8s *KubeController) DeleteDeploymentByName(deploymentName string) error {
	deploymentAPI := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	deletePolicy := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}

	if err := k8s.dynamicClient.Resource(deploymentAPI).Namespace(k8s.namespace).Delete(deploymentName, &deleteOptions); err != nil {
		return err
	}
	return nil
}

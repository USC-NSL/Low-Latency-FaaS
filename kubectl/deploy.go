package kubectl

import (
	"fmt"
	"strconv"
	"time"

	utils "github.com/USC-NSL/Low-Latency-FaaS/utils"
	glog "github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Defines all constants.
const kDockerhubUser string = "ch8728847"
const kNFImage string = "nf:debug"
const kCoopSchedImage string = "coopsched:debug"
const kFaaSControllerPort string = "10515"

var kFaaSCluster *utils.Cluster = nil
var kFaaSControllerIP string = ""

func SetFaaSClusterInfo(cluster *utils.Cluster) {
	kFaaSCluster = cluster
	kFaaSControllerIP = cluster.Master.IP
}

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
	"bypass":   "Bypass",
}

// Create an NF instance with type |funcType| on node |nodeName|,
// also assign the port |hostPort| of the host for the instance to receive gRPC requests.
// In Kubernetes, the instance is run as a deployment with name "nodeName-funcType-portId".
func (k8s *KubeController) makeDPDKDeploymentSpec(nodeName string,
	funcType string, hostPort int, pcie string, isPrimary bool, isIngress bool, isEgress bool, vPortIncIdx int, vPortOutIdx int) unstructured.Unstructured {
	if kFaaSControllerIP == "" {
		glog.Errorf("kubectl isn't aware of FaaS master node's IP. RPCs from containers will fail to reach the master node.")
	}

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
								// Hugepage requests equal the limts if
								// limits are specified but requests are not.
								"resources": map[string]interface{}{
									"limits": map[string]interface{}{
										"memory":        "128Mi",
										"hugepages-2Mi": "128Mi",
									},
								},
								"name":            funcType,
								"image":           kDockerhubUser + "/" + kNFImage,
								"imagePullPolicy": "IfNotPresent",
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
									"--primary=" + strconv.FormatBool(isPrimary),
									"--ingress=" + strconv.FormatBool(isIngress),
									"--egress=" + strconv.FormatBool(isEgress),
									"--isolation_key=" + pcie,
									"--device=" + pcie,
									"--vport_inc_idx=" + vPortInc,
									"--vport_out_idx=" + vPortOut,
									"--faas_grpc_server=" + kFaaSControllerIP + ":" + kFaaSControllerPort,
									"--monitor_grpc_server=" + kFaaSControllerIP + ":" + kFaaSControllerPort,
									"--redis_ip=128.105.144.32",
									"--redis_port=6380",
									"--redis_password=faas-nfv-cool",
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

// Creates a CooperativeSched instance on the worker node |nodeName|,
// In Kubernetes, the instance is run as a deployment with name "nodeName-sched".
func (k8s *KubeController) makeSchedDeploymentSpec(nodeName string, hostPort int) unstructured.Unstructured {
	deploymentName := fmt.Sprintf("%s-coopsched", nodeName)
	coreNum := "15"
	// w.Cores is the total number of available cores in the worker.
	// Note: One core is required to run gRPC and monitoring threads,
	// and cannot be used for NF threads.
	for _, w := range kFaaSCluster.Workers {
		if w.Name == nodeName {
			coreNum = fmt.Sprintf("%d", w.Cores-1)
		}
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
								// Hugepage requests equal the limts if
								// limits are specified but requests are not.
								"resources": map[string]interface{}{
									"limits": map[string]interface{}{
										"memory": "50Mi",
									},
								},
								"name":            "sched",
								"image":           kDockerhubUser + "/" + kCoopSchedImage,
								"imagePullPolicy": "IfNotPresent",
								"ports": []map[string]interface{}{
									{
										// The ports between [50052, 51051] on the host is used
										// for instance to receive gRPC requests.
										"containerPort": 10515,
										"hostPort":      hostPort,
									},
								},
								"command": []string{
									"/app/cooperative_sched",
									"--cores=" + coreNum,
									"--cli=0",
									"--logtostderr=1",
								},
							},
						}, // Ends containers
						"nodeName": nodeName,
					},
				},
			},
		},
	}

	return deployment
}

// Creates a CooperativeSched instance on node |nodeName|. Assigns
// TCP port |hostPort| to the instance.
func (k8s *KubeController) CreateSchedDeployment(nodeName string, hostPort int) (string, error) {
	api := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	deploy := k8s.dynamicClient.Resource(api).Namespace(k8s.namespace)

	spec := k8s.makeSchedDeploymentSpec(nodeName, hostPort)
	deploymentName := fmt.Sprintf("%s-coopsched", nodeName)

	_, err := deploy.Create(&spec, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}

	return deploymentName, nil
}

// Create an NF instance with type |funcType| on node |nodeName| at core |workerCore|,
// also assign the port |hostPort| of the host for the instance to receive gRPC requests.
// (Try for at most 20 seconds.)
func (k8s *KubeController) CreateDeployment(nodeName string,
	funcType string, hostPort int, pcie string, isPrimary bool, isIngress bool, isEgress bool, vPortIncIdx int, vPortOutIdx int) (string, error) {
	api := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	deploy := k8s.dynamicClient.Resource(api).Namespace(k8s.namespace)

	spec := k8s.makeDPDKDeploymentSpec(nodeName, funcType, hostPort, pcie, isPrimary, isIngress, isEgress, vPortIncIdx, vPortOutIdx)
	deploymentName := fmt.Sprintf("%s-%s-%s", nodeName, funcType, strconv.Itoa(hostPort))

	var err error
	start := time.Now()
	for time.Now().Unix()-start.Unix() < 20 {
		_, err = deploy.Create(&spec, metav1.CreateOptions{})
		if err == nil { // Successful
			glog.Infof("Deploy instance [%s] (pcie=%s,ingress=%v,egress=%v) on %s with port %d.\n",
				funcType, pcie, isIngress, isEgress, nodeName, hostPort)
			return deploymentName, nil
		}
	}
	glog.Errorf("Failed to deploy instance [%s] (pcie=%s,ingress=%v,egress=%v) on %s with port %d. %v\n",
		funcType, pcie, isIngress, isEgress, nodeName, hostPort, err)
	return "", err
}

// Delete a kubernetes deployment with the name |deploymentName|.
func (k8s *KubeController) DeleteDeployment(deploymentName string) error {
	api := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	deletePolicy := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}

	if err := k8s.dynamicClient.Resource(api).Namespace(k8s.namespace).Delete(deploymentName, &deleteOptions); err != nil {
		return err
	}
	return nil
}

package utils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

type Cluster struct {
	Master  ClusterNode   `json:"master"`
	Ofctl   ClusterNode   `json:"ofctl"`
	Workers []ClusterNode `json:"workers"`
}

type ClusterNode struct {
	Name       string   `json:"nodeName"`
	IP         string   `json:"IP"`
	Password   string   `json:"password"`
	PCIe       []string `json:"PCIe"`
	Cores      int      `json:"CPU"`
	SwitchPort int      `json:"switchPort"`
}

func ParseClusterInfo(fileName string) (*Cluster, error) {
	jsonFile, err := os.Open(fileName)
	defer jsonFile.Close()
	if err != nil {
		return nil, err
	}

	byteVal, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		return nil, err
	}

	var cluster Cluster
	json.Unmarshal(byteVal, &cluster)

	fmt.Printf("FaaS NFV cluster:\n")
	fmt.Printf(" - master node: name=%s, IP=%s\n", cluster.Master.Name, cluster.Master.IP)
	fmt.Printf(" - ofctl node: name=%s, IP=%s\n", cluster.Ofctl.Name, cluster.Ofctl.IP)
	fmt.Printf(" - total %d workers:\n", len(cluster.Workers))
	for i := 0; i < len(cluster.Workers); i++ {
		fmt.Printf("   - worker[%d]: name=%s, IP=%s, %d available VFs, switch port=%d\n", i, cluster.Workers[i].Name, cluster.Workers[i].IP, len(cluster.Workers[i].PCIe), cluster.Workers[i].SwitchPort)
	}

	return &cluster, nil
}

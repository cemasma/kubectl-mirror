package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"path/filepath"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type pod struct {
}

func main() {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(Optional) Absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "Absolute path to the kubeconfig file")
	}
	podName := flag.String("pod", "", "Pod name to clone")
	flag.Parse()

	if *podName == "" {
		fmt.Print("Pod name cannot be empty")
		return
	}

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	clientCfg, _ := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	namespace := clientCfg.Contexts[clientCfg.CurrentContext].Namespace

	if namespace == "" {
		namespace = "default"
	}

	podDetail, err := clientset.CoreV1().Pods(namespace).Get(context.Background(), *podName, metav1.GetOptions{})
	if err != nil {
		log.Fatalf("Error fetching pod: %v", err)
	}

	clonePodName := podDetail.Name + "-clone"
	clonePod := podDetail.DeepCopy()
	clonePod.Name = clonePodName
	clonePod.Labels["cloned"] = "true"
	clonePod.Labels["app"] = clonePod.Labels["app"] + "-clone"
	clonePod.ResourceVersion = ""
	clonePod.OwnerReferences = nil
	clonePod.UID = ""
	clonePod.Status.PodIP = ""
	clonePod.Status.PodIPs = nil
	clonePod.Status = v1.PodStatus{}
	clonePod.ObjectMeta.OwnerReferences = nil
	clonePod.ObjectMeta.ManagedFields = nil

	_, err = clientset.CoreV1().Pods(namespace).Create(context.TODO(), clonePod, metav1.CreateOptions{})
	if err != nil {
		panic(err.Error())
	}

	fmt.Println("Clone pod successfully created.")

	sourceIP := podDetail.Status.PodIP
	var destinationIP string
	for true {
		clonePodDetail, err := clientset.CoreV1().Pods(namespace).Get(context.Background(), clonePodName, metav1.GetOptions{})
		if err != nil {
			log.Fatalf("Error fetching pod: %v", err)
		}

		if len(clonePodDetail.Status.PodIP) > 0 {
			destinationIP = clonePodDetail.Status.PodIP
			break
		}
	}

	fmt.Printf("source %s\t%s", sourceIP, destinationIP)

	agentPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clonePodName + "-agent",
			Namespace: namespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				v1.Container{
					Name:  "agent",
					Image: "cemasma/kubectl-mirror:agent-1.0.0",
					Env: []v1.EnvVar{
						{Name: "SOURCE_IP", Value: sourceIP},
						{Name: "SOURCE_PORT", Value: "8080"},
						{Name: "DESTINATION_IP", Value: destinationIP},
						{Name: "DESTINATION_PORT", Value: "8080"},
					},
					SecurityContext: &v1.SecurityContext{
						Privileged: newTrue(),
					},
				},
			},
			HostNetwork: true,
		},
	}

	_, err = clientset.CoreV1().Pods(namespace).Create(context.Background(), agentPod, metav1.CreateOptions{})
	if err != nil {
		panic(err)
	}

	for {

	}
}

func newTrue() *bool {
	b := true
	return &b
}

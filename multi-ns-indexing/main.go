package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	// Get the user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	// Build the full path to the kubeconfig file
	kubeconfigPath := filepath.Join(homeDir, ".kube", "config")
	fmt.Println("kubeconfigPath:", kubeconfigPath)

	// Load kubeconfig file
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("config:[%+v]\n", config)

	// Create Kubernetes clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("clientset:[%+v]\n", clientset)

	// Define the namespaces to watch
	namespaces := []string{"msn", "kube-system", "kube-public", "default"} // List of namespaces

	// Loop over the namespaces and create informers for each
	for _, namespace := range namespaces {
		// Create an informer factory for each namespace
		informerFactory := informers.NewSharedInformerFactoryWithOptions(clientset, time.Minute*10,
			informers.WithNamespace(namespace))

		// Get the Pod informer for the specific namespace
		podInformer := informerFactory.Core().V1().Pods().Informer()

		// Add an indexer to index Pods by NodeName
		podInformer.AddIndexers(cache.Indexers{
			"nodeName": func(obj interface{}) ([]string, error) {
				pod := obj.(*corev1.Pod)
				return []string{pod.Spec.NodeName}, nil
			},
		})

		// Start the informer
		stopCh := make(chan struct{})
		defer close(stopCh)
		informerFactory.Start(stopCh)

		// Wait for cache sync
		informerFactory.WaitForCacheSync(stopCh)

		// Querying the cache for Pods running on a specific Node
		nodeName := "lima-rancher-desktop"
		indexer := podInformer.GetIndexer()

		// Retrieve Pods based on the index (nodeName)
		pods, err := indexer.ByIndex("nodeName", nodeName)
		if err != nil {
			panic(err)
		}

		// Print the names of the Pods running on the specific Node in each namespace
		fmt.Printf("Pods in namespace: %s\n", namespace)
		for _, pod := range pods {
			fmt.Printf("Pod Name: %s\n", pod.(*corev1.Pod).Name)
		}
	}
}

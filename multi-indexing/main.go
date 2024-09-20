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

	// namespace name to add the index for it.
	namespace := "msn"
	//namespace := "kube-system"

	// Create an informer factory to watch Pods
	//informerFactory := informers.NewSharedInformerFactory(clientset, 0)
	informerFactory := informers.NewSharedInformerFactoryWithOptions(clientset, time.Minute*10,
		informers.WithNamespace(namespace))
	fmt.Printf("informerFactory:[%+v]\n", informerFactory)

	// Get the Pod informer
	podInformer := informerFactory.Core().V1().Pods().Informer()
	fmt.Printf("podInformer:[%+v]\n", podInformer)

	// Add an indexer to index Pods by NodeName
	podInformer.AddIndexers(cache.Indexers{
		// Add indexer for a specific condiation or field
		"nodeName": func(obj interface{}) ([]string, error) {
			pod := obj.(*corev1.Pod)
			if pod.Spec.NodeName == "" {
				return []string{}, nil
			}
			return []string{pod.Spec.NodeName}, nil
		},
		// Add indexer for Labels
		"appLabel": func(obj interface{}) ([]string, error) {
			pod := obj.(*corev1.Pod)
			labelValue, exists := pod.Labels["msn_key"]
			if !exists {
				// If the "app" label does not exist, return an empty slice
				return []string{}, nil
			}
			// Return the value of the "app" label as a slice
			return []string{labelValue}, nil
		},
		// Add indexer for Annotations
		"ownerAnnotation": func(obj interface{}) ([]string, error) {
			pod := obj.(*corev1.Pod)
			annotationValue, exists := pod.Annotations["owner"]
			if !exists {
				// If the annotation does not exist, return an empty slice
				return []string{}, nil
			}
			// Return the annotation value as a slice
			return []string{annotationValue}, nil
		},
	})

	// Start the informer
	stopCh := make(chan struct{})
	defer close(stopCh)
	informerFactory.Start(stopCh)

	// Wait for cache sync
	informerFactory.WaitForCacheSync(stopCh)
	indexer := podInformer.GetIndexer()

	// Example 1: Querying Pods by a specific Node
	nodeName := "lima-rancher-desktop"
	pods, err := indexer.ByIndex("nodeName", nodeName)
	if err != nil {
		panic(err)
	}
	fmt.Printf("\nPods with nodeName=%s field:\n", nodeName)
	for _, pod := range pods {
		fmt.Printf("- %s\n", pod.(*corev1.Pod).Name)
	}

	// Example 2: Query Pods by app label
	appLabelValue := "msn_value"
	pods, err = podInformer.GetIndexer().ByIndex("appLabel", appLabelValue)
	if err != nil {
		panic(err)
	}
	fmt.Printf("\nPods with app=%s label:\n", appLabelValue)
	for _, pod := range pods {
		fmt.Printf("- %s\n", pod.(*corev1.Pod).Name)
	}

	// Example 3: Query Pods by revision annotation
	annotationValue := "msn"
	pods, err = podInformer.GetIndexer().ByIndex("ownerAnnotation", annotationValue)
	if err != nil {
		panic(err)
	}
	fmt.Printf("\nPods with ownerAnnotation=%s annotation:\n", annotationValue)
	for _, pod := range pods {
		fmt.Printf("- %s\n", pod.(*corev1.Pod).Name)
	}
	fmt.Println("\n\nNOTE: Please make sure to run the 'kubectl apply -f yamls' command if not ran already\n")
}

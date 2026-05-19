package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/client-go/util/workqueue" // 1. Import the workqueue package
)

const NodeIndex = "byNode"

// 2. Define a Worker struct to manage the queue and processing
type Worker struct {
	queue  workqueue.RateLimitingInterface
	clientset kubernetes.Interface
}

// NewWorker creates a new worker
func NewWorker(queue workqueue.RateLimitingInterface, clientset kubernetes.Interface) *Worker {
	return &Worker{
		queue:     queue,
		clientset: clientset,
	}
}

// Run starts the worker loop
func (w *Worker) Run(ctx context.Context, workerCount int) {
	// Spawn multiple worker goroutines (e.g., 3 workers)
	for i := 0; i < workerCount; i++ {
		go w.processNextItem(ctx)
	}
}

// processNextItem is the main loop for a single worker
func (w *Worker) processNextItem(ctx context.Context) {
	for {
		// 3. Block until an item is available in the queue
		item, shutdown := w.queue.Get()
		if shutdown {
			return
		}

		// Use a defer to ensure the item is marked as done (even if it panics)
		defer w.queue.Done(item)

		// 4. Process the item (The "Heavy Lifting")
		// Cast the item back to string (the namespace/name key)
		key, ok := item.(string)
		if !ok {
			// If the item isn't a string, just mark it as done and move on
			fmt.Printf("Item is not a string: %v\n", item)
			w.queue.Done(item)
			continue
		}

		// SIMULATE HEAVY WORK (e.g., talking to DB, API, or complex logic)
		fmt.Printf("[WORKER] Processing: %s... (Simulating 2s delay)\n", key)
		time.Sleep(2 * time.Second) 

		// 5. Handle Success
		// If successful, mark the item as fully processed
		w.queue.Forget(item)
		fmt.Printf("[WORKER] Finished: %s\n", key)
		
		// In a real app, you might update the object status here
	}
}

func main() {
	// --- Setup Clientset (Same as before) ---
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	// --- Setup WorkQueue ---
	// Create a rate-limiting queue (handles backoff on failures automatically)
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	
	// Create the Worker
	worker := NewWorker(queue, clientset)
	
	// Start the worker loop (spawn 3 workers)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker.Run(ctx, 3) 

	// --- Setup Informer Factory ---
	factory := informers.NewSharedInformerFactory(clientset, 0) // 0 = no resync by default
	podInformer := factory.Core().V1().Pods()

	// --- Add Indexer (Same as before) ---
	err = podInformer.Informer().AddIndexers(cache.Indexers{
		NodeIndex: func(obj interface{}) ([]string, error) {
			pod, ok := obj.(*corev1.Pod)
			if !ok {
				return nil, fmt.Errorf("not a pod")
			}
			if pod.Spec.NodeName == "" {
				return nil, nil
			}
			return []string{pod.Spec.NodeName}, nil
		},
	})
	if err != nil {
		panic(err)
	}

	// --- 6. Register Event Handlers (The "Observe" Phase) ---
	// Instead of doing work, we just add the KEY to the queue
	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
			
			// "Fire and forget" - just push to queue and exit immediately
			fmt.Printf("[HANDLER] Received ADD for: %s (Pushing to queue)\n", key)
			queue.Add(key) 
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			newPod := newObj.(*corev1.Pod)
			key := fmt.Sprintf("%s/%s", newPod.Namespace, newPod.Name)
			
			fmt.Printf("[HANDLER] Received UPDATE for: %s (Pushing to queue)\n", key)
			queue.Add(key)
		},
		DeleteFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
			
			fmt.Printf("[HANDLER] Received DELETE for: %s (Pushing to queue)\n", key)
			queue.Add(key)
		},
	})

	// --- Start Informer ---
	stopCh := make(chan struct{})
	defer close(stopCh)
	
	fmt.Println("Starting Pod Informer...")
	factory.Start(stopCh)

	// Wait for cache sync
	if !cache.WaitForCacheSync(stopCh, podInformer.Informer().HasSynced) {
		panic("Timed out waiting for caches to sync")
	}
	fmt.Println("Cache synced successfully! Workers are now processing events.")

	// Keep main goroutine alive
	<-stopCh
}

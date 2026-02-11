/*
Copyright (c) 2026 GeneClackman
SPDX-License-Identifier: MIT
*/

/*
Hortator API Gateway — OpenAI-compatible endpoint for agent orchestration.

This service translates OpenAI chat completion requests into AgentTask CRDs,
watches their lifecycle, and streams results back to the client.

Architecture:

	Client → POST /v1/chat/completions → Gateway → AgentTask CRD → Operator → Agent hierarchy
	Client ← SSE stream / JSON response ← Gateway ← Watch AgentTask status

Thread Continuity Roadmap:

	Level 0 (current): Stateless. Every request creates a fresh AgentTask.
	Level 1 (planned): Session-scoped. X-Hortator-Session header → reusable PVC
	  at /memory, accumulates context across requests. Session has TTL.
	  Implementation: Gateway maps session ID to a PVC name, sets
	  spec.storage.retain=true and a session label. Subsequent requests with
	  the same session ID mount the existing PVC.
	Level 2 (deferred): Full thread with server-side message history,
	  automatic context management, and summarization. Only build if demand
	  materializes. Would require a persistence layer (Postgres) for message
	  storage and a context window manager.
*/
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/michael-niemand/Hortator/internal/gateway"
)

func main() {
	var (
		addr       = flag.String("addr", ":8080", "Listen address")
		namespace  = flag.String("namespace", "", "Namespace to create AgentTasks in (required)")
		kubeconfig = flag.String("kubeconfig", "", "Path to kubeconfig (uses in-cluster config if empty)")
		authSecret = flag.String("auth-secret", "hortator-gateway-auth", "Name of Secret containing API keys")
		logLevel   = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	)
	flag.Parse()

	// Set up logging
	opts := zap.Options{}
	if *logLevel == "debug" {
		opts.Development = true
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	log := ctrl.Log.WithName("gateway")

	if *namespace == "" {
		// Fall back to env var, then current namespace from service account
		*namespace = os.Getenv("HORTATOR_NAMESPACE")
		if *namespace == "" {
			ns, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
			if err != nil {
				log.Error(err, "namespace is required: use --namespace, HORTATOR_NAMESPACE, or run in-cluster")
				os.Exit(1)
			}
			*namespace = string(ns)
		}
	}

	// Build K8s client
	var config *rest.Config
	var err error
	if *kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		log.Error(err, "failed to build k8s config")
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Error(err, "failed to create k8s clientset")
		os.Exit(1)
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Error(err, "failed to create dynamic client")
		os.Exit(1)
	}

	// Build gateway handler
	gw := &gateway.Handler{
		Namespace:  *namespace,
		Clientset:  clientset,
		DynClient:  dynClient,
		AuthSecret: *authSecret,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", gw.ChatCompletions)
	mux.HandleFunc("/v1/models", gw.ListModels)
	mux.HandleFunc("/api/v1/tasks/", gw.TaskArtifacts)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	})

	srv := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // Disabled for SSE streaming
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("starting gateway", "addr", *addr, "namespace", *namespace)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(err, "server failed")
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

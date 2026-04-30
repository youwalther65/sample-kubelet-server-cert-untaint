/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"flag"
	"os"
	"time"

	"k8s.io/klog/v2"
)

const (
	TaintWatcherDuration           = 30 * time.Second
	KubeletServerCertCheckDuration = 75 * time.Second
	DefaultCheckIntervalSec        = 5
	DefaultTaintKey                = "example.com/kubelet-no-server-cert"
	DefaultKubeletCertFile         = "/var/lib/kubelet/pki/kubelet-server-current.pem"

	envNodeName      = "NODE_NAME"
	envTaintKey      = "TAINT_KEY"
	envCheckInterval = "CHECK_INTERVAL"
)

type Config struct {
	TaintKey       string
	NodeName       string
	CheckInterval  time.Duration
	KubeconfigPath string
	SkipCertCheck  bool
}

// These are set during build time via -ldflags.
var (
	taintremoverVersion string
	gitCommit           string
	buildDate           string
	checkIntervalSec    int
)

func Load() *Config {
	cfg := &Config{}

	klog.InitFlags(nil)
	flag.StringVar(&cfg.TaintKey, "taint-key", os.Getenv(envTaintKey), "The taint key to watch for and remove")
	flag.StringVar(&cfg.NodeName, "node-name", os.Getenv(envNodeName), "The nodename to watch for and remove the taint from")
	flag.IntVar(&checkIntervalSec, "check-interval", DefaultCheckIntervalSec, "The interval to check for kubelet certificate")
	flag.StringVar(&cfg.KubeconfigPath, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.BoolVar(&cfg.SkipCertCheck, "skip-cert-check", false, "Skip waiting for kubelet server certificate")

	flag.Parse()

	klog.V(2).InfoS("Starting taint remover", "Version", taintremoverVersion, "GitCommit", gitCommit, "BuildDate", buildDate)

	if cfg.TaintKey == "" {
		klog.V(2).InfoS("No taint-key specified - using default", "taint", DefaultTaintKey)
		cfg.TaintKey = DefaultTaintKey
	}

	if cfg.NodeName == "" {
		klog.ErrorS(nil, "node-name flag or NODE_NAME environment variable is required")
		os.Exit(1)
	}

	cfg.CheckInterval = time.Duration(checkIntervalSec) * time.Second
	klog.V(2).InfoS("Configuration", "node", cfg.NodeName, "taint", cfg.TaintKey, "check-interval", checkIntervalSec, "skip-cert-check", cfg.SkipCertCheck)

	return cfg
}

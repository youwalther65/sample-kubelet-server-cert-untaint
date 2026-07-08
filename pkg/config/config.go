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
	DefaultTaintWatcherSec           = 30
	DefaultKubeletServerCertCheckSec = 75
	DefaultCheckIntervalSec          = 5
	DefaultStartupJitterSec          = 3
	DefaultTaintKey                  = "example.com/kubelet-no-server-cert"
	DefaultKubeletCertFile           = "/var/lib/kubelet/pki/kubelet-server-current.pem"

	envNodeName      = "NODE_NAME"
	envTaintKey      = "TAINT_KEY"
	envCheckInterval = "CHECK_INTERVAL"
)

type Config struct {
	TaintKey                       string
	NodeName                       string
	CheckInterval                  time.Duration
	StartupJitter                  time.Duration
	TaintWatcherDuration           time.Duration
	KubeletServerCertCheckDuration time.Duration
	KubeconfigPath                 string
	SkipCertCheck                  bool
}

// These are set during build time via -ldflags.
var (
	taintremoverVersion string
	gitCommit           string
	buildDate           string
	checkIntervalSec    int
	startupJitterSec    int
	taintWatcherSec     int
	certCheckSec        int
)

func Load() *Config {
	cfg := &Config{}

	klog.InitFlags(nil)
	flag.StringVar(&cfg.TaintKey, "taint-key", os.Getenv(envTaintKey), "The taint key to watch for and remove")
	flag.StringVar(&cfg.NodeName, "node-name", os.Getenv(envNodeName), "The nodename to watch for and remove the taint from")
	flag.IntVar(&checkIntervalSec, "check-interval", DefaultCheckIntervalSec, "The interval to check for kubelet certificate")
	flag.IntVar(&startupJitterSec, "startup-jitter", DefaultStartupJitterSec, "Max random delay in seconds before contacting the apiserver, to spread DaemonSet boot load at scale (0 disables)")
	flag.IntVar(&taintWatcherSec, "taint-watcher-duration", DefaultTaintWatcherSec, "Overall wall-clock budget in seconds for removing the taint before giving up")
	flag.IntVar(&certCheckSec, "cert-check-duration", DefaultKubeletServerCertCheckSec, "Max time in seconds to wait for the kubelet server certificate to appear")
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
	if startupJitterSec < 0 {
		startupJitterSec = 0
	}
	cfg.StartupJitter = time.Duration(startupJitterSec) * time.Second

	if taintWatcherSec <= 0 {
		klog.V(2).InfoS("Invalid taint-watcher-duration - using default", "given", taintWatcherSec, "default", DefaultTaintWatcherSec)
		taintWatcherSec = DefaultTaintWatcherSec
	}
	cfg.TaintWatcherDuration = time.Duration(taintWatcherSec) * time.Second

	if certCheckSec <= 0 {
		klog.V(2).InfoS("Invalid cert-check-duration - using default", "given", certCheckSec, "default", DefaultKubeletServerCertCheckSec)
		certCheckSec = DefaultKubeletServerCertCheckSec
	}
	cfg.KubeletServerCertCheckDuration = time.Duration(certCheckSec) * time.Second

	klog.V(2).InfoS("Configuration", "node", cfg.NodeName, "taint", cfg.TaintKey, "check-interval", checkIntervalSec, "startup-jitter", startupJitterSec, "taint-watcher-duration", taintWatcherSec, "cert-check-duration", certCheckSec, "skip-cert-check", cfg.SkipCertCheck)

	return cfg
}

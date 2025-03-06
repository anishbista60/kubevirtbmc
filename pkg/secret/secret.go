package secret

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// AuthCredentials holds the IPMI authentication credentials
type AuthCredentials struct {
	Username string
	Password string
}

// SecretManager handles authentication credential management using Kubernetes secrets
type SecretManager struct {
	context         context.Context
	secretNamespace string
	secretName      string
	k8sClient       kubernetes.Interface
	authSecret      *AuthCredentials
	secretInformer  cache.SharedIndexInformer
}

var secretManager *SecretManager

// SetSecretManager assigns the global secret manager instance.
func SetSecretManager(sm *SecretManager) {
	secretManager = sm
}

// GetCredentials returns the current username and password using the global secret manager.
func GetCredentials() (string, string) {
	if secretManager == nil {
		return "", ""
	}
	return secretManager.GetCredentials()
}

// NewSecretManager creates a new SecretManager with the given secret reference
func NewSecretManager(ctx context.Context, secretRef string) (*SecretManager, error) {
	// Parse secret reference in format "namespace/name"
	parts := strings.Split(secretRef, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid secret reference format, expected namespace/name, got: %s", secretRef)
	}
	secretNamespace, secretName := parts[0], parts[1]

	// Create Kubernetes client
	k8sClient, err := getKubernetesClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %v", err)
	}

	return &SecretManager{
		context:         ctx,
		secretNamespace: secretNamespace,
		secretName:      secretName,
		k8sClient:       k8sClient,
		authSecret:      &AuthCredentials{},
	}, nil
}

// getKubernetesClient creates a Kubernetes client using in-cluster config
func getKubernetesClient() (kubernetes.Interface, error) {
	clientConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster config: %v", err)
	}
	return kubernetes.NewForConfig(clientConfig)
}

// Initialize sets up the secret watcher and fetches initial credentials
func (sm *SecretManager) Initialize() error {
	if sm.secretNamespace == "" || sm.secretName == "" {
		logrus.Info("No secret reference provided, IPMI authentication disabled")
		return nil
	}
	sm.fetchInitialCredentials()

	return sm.setupSecretWatcher()
}

// setupSecretWatcher creates an informer to watch for changes to the target secret
func (sm *SecretManager) setupSecretWatcher() error {
	factory := informers.NewSharedInformerFactoryWithOptions(
		sm.k8sClient,
		10*time.Second,
		informers.WithNamespace(sm.secretNamespace),
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.FieldSelector = fmt.Sprintf("metadata.name=%s", sm.secretName)
		}),
	)

	sm.secretInformer = factory.Core().V1().Secrets().Informer()
	sm.secretInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    sm.handleSecretChange,
		UpdateFunc: func(_, newObj interface{}) { sm.handleSecretChange(newObj) },
		DeleteFunc: func(_ interface{}) {
			logrus.Warn("Authentication secret was deleted, IPMI authentication may not work correctly")
			sm.authSecret.Username = ""
			sm.authSecret.Password = ""
		},
	})
	return nil
}

// handleSecretChange updates credentials when the secret changes
func (sm *SecretManager) handleSecretChange(obj interface{}) {
	secret := obj.(*corev1.Secret)
	if secret.Name == sm.secretName {
		sm.updateCredentials(secret)
	}
}

// updateCredentials extracts username and password from the secret and updates the credentials
func (sm *SecretManager) updateCredentials(secret *corev1.Secret) {
	username, hasUsername := secret.Data["username"]
	password, hasPassword := secret.Data["password"]

	if !hasUsername || !hasPassword {
		logrus.Warn("Secret does not contain required 'username' and 'password' keys")
		return
	}

	sm.authSecret.Username = string(username)
	sm.authSecret.Password = string(password)
	logrus.Infof("Updated IPMI authentication credentials from secret %s/%s", sm.secretNamespace, sm.secretName)
}

// fetchInitialCredentials gets the initial credentials from the secret
func (sm *SecretManager) fetchInitialCredentials() error {
	secret, err := sm.k8sClient.CoreV1().Secrets(sm.secretNamespace).Get(
		sm.context,
		sm.secretName,
		metav1.GetOptions{},
	)
	if err != nil {
		logrus.Warnf("Authentication secret %s/%s not found: %v", sm.secretNamespace, sm.secretName, err)
		return nil
	}
	sm.updateCredentials(secret)
	return nil
}

// Run starts the secret informer
func (sm *SecretManager) Run() error {
	if sm.secretInformer == nil {
		return nil
	}
	go sm.secretInformer.Run(sm.context.Done())
	if !cache.WaitForCacheSync(sm.context.Done(), sm.secretInformer.HasSynced) {
		return fmt.Errorf("timed out waiting for secret cache to sync")
	}
	logrus.Info("Secret manager is running")
	return nil
}

// GetCredentials returns the current credentials
func (sm *SecretManager) GetCredentials() (string, string) {
	return sm.authSecret.Username, sm.authSecret.Password
}

// Stop stops the secret manager
func (sm *SecretManager) Stop() {
	logrus.Info("Secret manager stopped")
}

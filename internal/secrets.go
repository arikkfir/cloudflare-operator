package internal

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

func GetSecretValue(ctx context.Context, c client.Client, defaultNamespace, secretName, secretKey string) (string, error) {
	var secretNamespace string

	secretTokens := strings.SplitN(secretName, "/", 2)
	if len(secretTokens) == 2 {
		secretNamespace = secretTokens[0]
		secretName = secretTokens[1]
	} else {
		secretNamespace = defaultNamespace
		secretName = secretTokens[0]
	}

	secret := &v1.Secret{}
	err := c.Get(ctx, client.ObjectKey{Namespace: secretNamespace, Name: secretName}, secret)
	if err != nil {
		return "", fmt.Errorf("secret '%s/%s' not found or inaccessible: %w", secretNamespace, secretName, err)
	}

	token, ok := secret.Data[secretKey]
	if !ok {
		return "", fmt.Errorf("failed to find key '%s' in secret '%s/%s': %w", secretKey, secretNamespace, secretName, err)
	}

	return string(token), nil
}

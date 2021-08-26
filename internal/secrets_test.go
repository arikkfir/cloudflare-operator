package internal

import (
	"context"
	"github.com/Pallinder/go-randomdata"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

func TestGetSecretValue(t *testing.T) {
	ctrl := gomock.NewController(t)

	secretNamespace := randomdata.SillyName()
	secretName := randomdata.SillyName()
	secretKey := randomdata.SillyName()

	var clientMock *MockClient
	var value string
	var err error

	clientMock = NewMockClient(ctrl)
	clientMock.EXPECT().
		Get(gomock.Any(), client.ObjectKey{Namespace: secretNamespace, Name: secretName}, gomock.Any()).
		Times(1).
		DoAndReturn(func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			secret := obj.(*v1.Secret)
			secret.Data = map[string][]byte{secretKey: []byte("value")}
			return nil
		})
	value, err = GetSecretValue(context.Background(), clientMock, secretNamespace, secretName, secretKey)
	assert.Nil(t, err)
	assert.Equal(t, value, "value")

	clientMock = NewMockClient(ctrl)
	clientMock.EXPECT().
		Get(gomock.Any(), client.ObjectKey{Namespace: secretNamespace, Name: secretName}, gomock.Any()).
		Times(1).
		DoAndReturn(func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			secret := obj.(*v1.Secret)
			secret.Data = map[string][]byte{secretKey: []byte("value")}
			return nil
		})
	value, err = GetSecretValue(context.Background(), clientMock, "default", secretNamespace+"/"+secretName, secretKey)
	assert.Nil(t, err)
	assert.Equal(t, value, "value")

	clientMock = NewMockClient(ctrl)
	clientMock.EXPECT().
		Get(gomock.Any(), client.ObjectKey{Namespace: secretNamespace, Name: secretName}, gomock.Any()).
		Times(1).
		DoAndReturn(func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			secret := obj.(*v1.Secret)
			secret.Data = map[string][]byte{"unknownKey": []byte("value")}
			return nil
		})
	value, err = GetSecretValue(context.Background(), clientMock, "default", secretNamespace+"/"+secretName, secretKey)
	assert.Error(t, err)
	assert.Empty(t, value)
}

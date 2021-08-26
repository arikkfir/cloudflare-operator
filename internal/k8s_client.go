package internal

//go:generate mockgen -destination k8s_client_mock.go -package internal sigs.k8s.io/controller-runtime/pkg/client Client

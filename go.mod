module github.com/arikkfir/cloudflare-operator

go 1.16

require (
	github.com/Pallinder/go-randomdata v1.2.0
	github.com/cloudflare/cloudflare-go v0.17.0
	github.com/go-logr/logr v0.4.0
	github.com/golang/mock v1.5.0
	github.com/onsi/gomega v1.10.2
	github.com/stretchr/testify v1.7.0
	go.uber.org/zap v1.15.0
	golang.org/x/sys v0.0.0-20210629170331-7dc0b73dc9fb // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
	k8s.io/api v0.19.2
	k8s.io/apimachinery v0.21.2
	k8s.io/client-go v0.19.2
	k8s.io/utils v0.0.0-20200912215256-4140de9c8800
	sigs.k8s.io/controller-runtime v0.7.2
)

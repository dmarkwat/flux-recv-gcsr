module github.com/dmarkwat/flux-gcsr

go 1.13

require github.com/fluxcd/flux v1.18.0

require (
	cloud.google.com/go/pubsub v1.3.0
	github.com/prometheus/common v0.7.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.4.0
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	google.golang.org/api v0.20.0
	google.golang.org/grpc v1.27.1
)

replace (
	github.com/docker/distribution => github.com/fluxcd/distribution v0.0.0-20190419185413-6c9727e5e5de
	github.com/docker/docker => github.com/docker/docker v0.7.3-0.20190327010347-be7ac8be2ae0
	github.com/fluxcd/flux/pkg/install => github.com/fluxcd/flux/pkg/install v0.0.0-20200306161357-1c4697fcade1
)

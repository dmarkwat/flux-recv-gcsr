## Flux GCSR receiver

Flux receiver for Google Cloud Source Repository Pubsub notifications.

When commits are pushed to GCSR, (and when configured) a pubsub message is emitted with details about the commit pushed (see [here](https://cloud.google.com/source-repositories/docs/pubsub-notifications)).
This receiver, modeled after the official [flux-recv](https://github.com/fluxcd/flux-recv), uses these pubsub notifications to trigger flux sync operations.

Requirements for this to work:
- Pubsub notifications [configured](https://cloud.google.com/source-repositories/docs/configuring-notifications) (messages in json format) for the GCSR repo flux is using
- IAM needed to consume messages from the topic
- IAM needed to create Pubsub subscriptions (optional, for those wearing firm tin hats)
- Flux configured to communicate with GCSR (more below)
- Recommended: build this image and add it as a sidecar on the flux daemon

### Flux & GCSR

GCSR, like other GCP services, is easiest to work with when the `gcloud` command (and all the auth machinery that comes with it) is present.
GCSR doesn't have the same concept of "deploy keys" as GitHub or GitLab, and the SSH key support is bound to user accounts--not service accounts.
So, the easiest way to talk to GCSR using a service account is with the `gcloud` git credential integration.

To get this working on GKE:
- Enable [workload identity](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity) on the cluster
- Add `gcloud` to the flux image
```
FROM fluxcd/flux:1.18.0@sha256:8fcf24dccd7774b87a33d87e42fa0d9233b5c11481c8414fe93a8bdc870b4f5b
COPY --from=google/cloud-sdk:v281.0.0-slim@sha256:34af9772f9c0aee4051e717840ab7042648e5feb06a539c7857394e076edcce0 /usr/lib/google-cloud-sdk /opt/google-cloud-sdk
```
- Add the git credential helper to the gitconfig used by flux
```
[credential "https://source.developers.google.com"]
  helper = !/opt/google-cloud-sdk/bin/git-credential-gcloud.sh
  username = whateverworks
  email = whateverworks
```
- Deploy flux, configured for the GCSR repo
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: flux
spec:
  template:
    spec:
      containers:
      - name: flux
        args:
        ...
        - --git-url=https://source.developers.google.com/p/my-project/r/my-repository
```

With the above done, flux can now talk to GCSR and use this project as a sidecar without issue!



# orchestration 

general stuff per project
- tf
- charts


# vision
## 
`kest` cli

example session sketched out:

```zsh
$ cd ~/projects/example-service/
$ cat .kestconfig
helm:
	chart: "oci://ghcr.io/example-org/example-charts/example-ms:2.5.91"
	values_dir: misc/chart
	deploy-scripts: misc/chart/deploy-scripts/migrate.sh
terraform:
	iac_dir: misc/iac
$ ls misc/chart/
deploy-scripts/
shared.yaml
local.yaml
dev.yaml
$ ls misc/chart/deploy-scripts/
migrate.sh
$ ls misc/iac/live
dev/
stage/
prod/
$ ls -a misc/iac/live/dev/
.terraform-version
.terraform.lock.hcl
root.tf

$ kest --verbose -e dev terraform output
debug: using misc/iac/live/dev...
<appropriate terraform output command results>
$ kest -e dev helm deploy
debug: using misc/chart...
error: not in CI environment, no deploys!
<error: cannot deploy from dirty worktree>
<error: cannot deploy to prod from feature branch etc.>
$ kest -e dev helm deploy --force-from-laptop
debug: using misc/chart...
info: will run misc/chart/deploy-scripts/migrate.sh at the appropriate time
<effectively runs the correct actions script from the common-helm-chart repo that does our things etc. etc.>


$ echo "stretch feature idea:"
stretch feature idea:
$ kest deployment info
env: dev
last_deployed: <whatever date retrieved from helm>
app_version: 1.2.0
chart: oci:$whatever:hh-ms:2.5.91

env: stage
last_deployed: 
app_version: 1.1.0
chart: oci:$whatever:hh-ms:2.4.5

env: prod
etc.
```

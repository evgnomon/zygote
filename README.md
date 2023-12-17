# Zygote: Cloud Functions Runtime for Factories without Human!
Zygote is a handy runtime for developing cloud functions guided by the principle "No human is let in the factory". The function code gets seamlessly integrated using Zygote CI/CD which is offered as a service and could be added to your Github repo as an Application. This ensures you can get a function up and running "without entering the factory". Zygote is freely available under HGL License.

## Quick Start Guide
```bash
git clone git@github.com:evgnomon/zygote.git
go install .

cd examples/example-project-js

zygote add -z mysql:[VM1,VM2,VM3] -z app:[VM4] # Run app on a clustered mysql instance on port 80
zygote stop # Stop everything
zygote restart # Rest everything
zygote restart -z mysql:[VM1] # Rest VM1 while the cluster is operating
zygote restart -z app:[VM4,VM5] # Add new VM5 to the set
zygote restart -z mysql # Restart the mysql cluster
zygote replace -z mysql:[VM1]:[VM6] # Replace VM1 with VM6 for mysql cluster so the cluster will be [VM6,VM2,VM3] afterward
zygote add -z redis:[VM7,VM8,VM9] # Add a new redis instance to the running cluster
zygote rename -z redis:redis-1 # Rename redis to redis-1 keeping everything else
zygote add -z redis-2:[VM10,VM11,VM12] # Add a new redis shard to the running cluster
zygote fork -e stage # Fork everything to the stage environment (from the default env.) VM1-stage, VM2-stage would be machine names.
zygote switch -e stage # commands afterward target stage
zygote fork -e prod # create prod environment
zygote switch -e prod # switch to prod
zygote mount -z app --url www.example.app:80/myapp
zygote forget # Bye, nothing exists now!
```
Which runs the app on VM4 together with MySQL on VM1..3. The VMs are defined in ~/.ssh/config and it could be anywhere.

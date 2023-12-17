# Zygote: Cloud Functions Runtime for Factories without Human!
Zygote is a handy runtime for developing cloud functions guided by the principle "No human is let in the factory". The function code gets seamlessly integrated using Zygote CI/CD which is offered as a service and could be added to your Github repo as an Application. This ensures you can get a function up and running "without entering the factory". Zygote tool and the runtime is freely available under HGL License and could be quickly setup locally on your machine supporting Linux, macOS and Windows (Both native and WSL2).

Zygote has bultin support for MySQL InnoDB cluster as the default database and sharded Redis cluster as in-memory store and message stream which makes it self sufficient out of the box to quickly create the first scaleable HTTP function which both works locally, in the cloud and on-prem so gives the choice to you choosing the factory without entering it.

> In a world where "no human is let in the factory", machine is nothing than a domain name exposing a REST API (or a nice UI) with some paying subscribers. And eventually there would be no job left except sending pull requests to code repositories to modify the world wide machine, it is just a matter of time to reach there!

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
VMs are defined in ~/.ssh/config

<p align="center">
<img src="docs/assets/Zygote.svg" width="256" height="256">
</p>
<p align="center"> Zygote </p>
<p align="center"> A Function Runtime for Factories without Human </p>


> Machine is a name. An IO domain. Secure the exit right now as "no human is let in the factory" in 44 minutes. That factory prints everything including itself.

> Dec 2023

Subscribe to Zygote .Run and run your cloud function in there. Otherwise fork everything and run it on your own machines.
Guess you have all the codes to be able to run a fork. We protect our private keys. Not sure if we need to keep you behind any other gate.

Linux, macOS and Windows (Both native and WSL2) are supported for local development. But we just support Debian Linux in the server. Bultin support for MySQL InnoDB cluster as the general purpose database and a sharded Redis cluster as in-memory store and message bus. Just plug commodity machines and start using!

## Quick Start Guide
```bash
git clone git@github.com:evgnomon/zygote.git
go install .

cd examples/example-project-js

zygote add -z mysql:[1,2,3] -z app:[1] # Run app on a clustered mysql instance on port 80
zygote stop # Stop everything
zygote restart # Rest everything
zygote restart -z mysql:[1] # Rest [1] while the cluster is operating
zygote restart -z app:[4,5] # Add new VM5 to the set
zygote restart -z mysql # Restart the mysql cluster
zygote replace -z mysql:[1]:[6] # Replace VM1 with VM6 for mysql cluster so the cluster will be [6,2,3] afterward
zygote add -z redis:[7,8,9] # Add a new redis instance to the running cluster
zygote rename -z redis:redis-1 # Rename redis to redis-1 keeping everything else
zygote add -z redis-2:[10,11,12] # Add a new redis shard to the running cluster
zygote fork -e stage # Fork everything to the stage environment (from the default env.) 1-stage, 2-stage would be machine names.
zygote switch -e stage # commands afterward target stage
zygote fork -e prod # create prod environment
zygote switch -e prod # switch to prod
zygote mount -z app --url www.example.app:80/myapp
zygote forget # Bye, nothing exists now!
```
VMs are defined in ~/.ssh/config

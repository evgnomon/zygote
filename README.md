# Zygote: Cloud Functions Runtime for Factories without Human!
Zygote is a handy runtime for developing functions, essentially the building blocks of apps. These functions operate in the cloud when using Zygote. Guided by the principle "No human is let in the factory", the source code needs to be accessible in the cloud (our "factory") and on our laptops. This ensures we can make changes without directly entering the "factory". Zygote is freely available under the HGL License, making it accessible for our projects.

## Quick Start Guide
```bash
git clone git@github.com:evgnomon/zygote.git
go install .

cd examples/example-project-js

zygote run mysql:VM1,VM2,VM3 app:VM4 . # Run app on a clustered mysql instance
zygote stop # Stop everything
zygote restart # Rest everything
zygote restart mysql:VM1 # Rest VM1 while the cluster is operating
zygote restart app:VM4,VM5 # Add new VM5 to the set
zygote restart mysql # Restart the mysql cluster
```
Which runs the app on VM4 together with MySQL on VM1..3. The VMs are defined in ~/.ssh/config and it could be anywhere.

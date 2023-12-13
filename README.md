# Zygote: Cloud Functions Runtime for Factories without Human!

## Introduction
Zygote is a runtime useful for writing functions (aka making apps). Functions using Zygote just integrate and run in the cloud. There is just one covenant driving everything in Zygote "No human is let in the factory". The source must be available both inside the factory and on my laptop to keep the covenant forever. As no human is let in the factory, there is no connection between the factory and my laptop except when I want to change the code running the factory. So there could be more than one factory running the code, and my laptop is a the first place the next change to the factory happens inside, so it is a full function independent factory alone which there is no human is inside (as I am not inside my laptop). Zygote is distributed under HGL General License.

## Core Features
- **AI-Focused Applications**: Ideal for creating apps that leverage AI technology, offering APIs and human-readable interfaces.
- **Versatile Deployment**: Run your apps on your preferred platform, be it cloud-based or on local machines, ensuring you're always in control.
- **Customizable Components**: Configure Zygote with your own tools and resources, like APIs or open-source databases, for a tailored experience.

## Quick Start Guide
```bash
git clone git@github.com:evgnomon/zygote.git
go install .

cd MyApp
zygote -i X:azure Y:aws Z:vm1,vm2 run .
```
*X, Y, Z can be tailored to include your own resources, cloud services, or local VMs.*

## What Sets Zygote Apart
- **User-Controlled Integration**: You maintain full control over essential components (X, Y, Z) ensuring seamless operation across different environments.
- **AI Compatibility**: Designed for an AI-driven world, ensuring your application stays relevant and functional in rapidly evolving tech landscapes.

## Example Usage
```bash
zygote -i MySQL:aws .       # Launch on AWS
zygote stop                 # Stop the application
zygote -i MySQL:azure .     # Migrate to Azure with data transfer prompt
```

## Collaboration and Adaptability
As the developer of Zygote, I'm dedicated to continual enhancement, ensuring it remains a valuable tool in an AI-driven world. We support contributions and innovations from users and developers alike.

## Join the Zygote Community
Zygote is more than just a runtime; it's a gateway to the future of web application development in an AI-dominated era. We welcome users, developers, and cloud providers to explore and contribute to this evolving platform. Welcome to Zygote!

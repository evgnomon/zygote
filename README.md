# Zygote: Cloud Functions Runtime for Factories without Human!

## Introduction
Zygote is a handy runtime for developing functions, essentially the building blocks of apps. These functions operate in the cloud when using Zygote. Guided by the principle "No human is let in the factory", the source code needs to be accessible in the cloud (our "factory") and on our laptops. This ensures we can make changes without directly entering the "factory". Zygote is freely available under the HGL License, making it accessible for our projects.

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

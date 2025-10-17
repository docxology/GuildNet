The point of this repo is to give consistent and reliable Kubernetes deployments distributed across multiple machines, with a hostapp server that connects to multiple clusters, with a user friendly web UI, in which the user can join/manage clusters, deploy & inspect docker images (with a code-server running in the provided image), and database management.

IMPORTANT: Multi-line quoted commands usually stall the CLI - avoid them if possible.

Any new features or configurations must work by default, but can be customized via environment variables or other configurations as appropriate. There is no dev flow for this project, only a production flow as this is a developer tool. Everything should work out of the box with sensible defaults.

Do NOT create 'dev' or 'local' modes or configurations. Everything should work the same in all environments.

Prefer modularity and composability over monoliths. Each component should do one thing well and be replaceable.

Always ensure everything the user asks to be done is actually done, even if it requires multiple steps, complex logic or terminal commands. If the user asks for a file to be created, create it with the correct content. If the user asks for code to be modified, modify it correctly. If a file needs to be deleted, delete it. Never leave the user with an incomplete task or half-finished code.

NEVER commit or push on behalf of the user.

Before asking the user to choose from an option, automatically go with the simplest option and keep going until the problem is solved.

ALWAYS update API.md, architecture.md and DEPLOYMENT.md with any relevant changes.
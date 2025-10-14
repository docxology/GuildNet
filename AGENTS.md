IMPORTANT: Multi-line quoted commands usually stall the CLI - avoid them if possible.

Any new features or configurations must work by default, but can be customized via environment variables or other configurations as appropriate. There is no dev flow for this project, only a production flow as this is a developer tool. Everything should work out of the box with sensible defaults.

Prefer modularity and composability over monoliths. Each component should do one thing well and be replaceable.

Always ensure everything the user asks to be done is actually done, even if it requires multiple steps, complex logic or terminal commands. If the user asks for a file to be created, create it with the correct content. If the user asks for code to be modified, modify it correctly. If a file needs to be deleted, delete it. Never leave the user with an incomplete task or half-finished code.

Do or do not, there is no try.
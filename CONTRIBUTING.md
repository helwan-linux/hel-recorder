# Contributing to Helwan Screen Recorder

Thank you for your interest in contributing to Helwan Screen Recorder.

Helwan Screen Recorder is a lightweight, open-source screen and audio recording application for Linux, built with Go and GTK3.

We welcome contributions that improve the application's reliability, usability, performance, and maintainability.

## Development Environment

Helwan Screen Recorder is designed for Linux/X11.

Required development tools:

- Go
- GTK3 development libraries
- FFmpeg
- X11 development libraries

## Building from Source

Clone the repository:

```bash
git clone https://github.com/helwan-linux/hel-recorder.git
cd hel-recorder/hel-recorder
```

Build the application:

```bash
go build -o hel-recorder .
```

Run:

```bash
./hel-recorder
```

## Contribution Guidelines

- Keep changes simple, focused, and easy to maintain.
- Avoid unnecessary dependencies.
- Preserve compatibility with the project's Linux/X11 environment.
- Test changes before submitting them.
- Update documentation when functionality changes.
- Avoid introducing features that add complexity without practical value.

## Reporting Bugs

When reporting a bug, please provide:

- A clear description of the issue.
- Steps to reproduce it.
- Your Linux distribution and desktop environment.
- Relevant error messages or terminal output.

## Suggesting Features

Feature requests should describe the problem being solved and explain the practical benefit of the proposed feature.

Suggestions should align with the project's design goals: simplicity, reliability, practicality, and lightweight operation.

## Pull Requests

Before submitting a pull request:

- Explain the purpose of your changes.
- Keep each pull request focused.
- Test the affected functionality.
- Include relevant documentation updates.

## Project Philosophy

Helwan Screen Recorder is built around a simple principle:

**Record. Pause. Resume. Stop.**

The goal is to provide a dependable and straightforward recording experience without unnecessary complexity.

Thank you for contributing.

---

**Helwan Screen Recorder**  
Part of **Helwan Linux**

https://github.com/helwan-linux/hel-recorder

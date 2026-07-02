# corebuddy

A lightweight, zero-bloat interactive TUI CPU monitor and core diagnostics tool for Linux. 

No background daemons, no heavy dependencies—just pure, real-time parsing of Linux kernel files (`/proc` and `/sys`) wrapped in a responsive terminal interface.

---

## 🛠️ Step-by-Step Installation Guide

Follow these steps to set up Go, configure your environment, and install `corebuddy` system-wide.

### Step 1: Install Go Compiler
If you don't have Go installed on your Linux system, install it using your package manager:

* **Arch Linux / Arch based:**
  ```
  sudo pacman -S go

  Debian / Ubuntu / Mint:

    sudo apt update && sudo apt install golang -y

    Alpine Linux:

    sudo apk add go

    Void Linux:

    sudo xbps-install -S go

Verify the installation by running go version.

Step 2: Configure Environment Variables (Crucial)

When you install packages via go install, Go places the executable binaries inside the ~/go/bin directory. For your terminal to recognize the corebuddy command from anywhere, you must add this directory to your system's $PATH.

    Open your shell configuration file (.bashrc, .zshrc, or .bash_profile) using a text editor (e.g., nano, vim):

    nano ~/.bashrc

    (If you use Zsh, replace ~/.bashrc with ~/.zshrc)

    Append the following lines to the very bottom of the file

    # Go Binaries Path
    export GOPATH=$HOME/go
    export PATH=$PATH:$GOPATH/bin

    Save the file and reload your terminal configuration to apply the changes immediately:

    source ~/.bashrc 

    
   Step 3: Install corebuddy

Now that Go and your PATH are properly configured, install corebuddy directly from GitHub using the official command:
Bash

go install [github.com/odxnoj-sh/corebuddy@latest](https://github.com/odxnoj-sh/corebuddy@latest)

Go will automatically download, compile, and place the executable inside your ~/go/bin/ folder.

🏃 Usage

Once the installation is complete, close your current terminal or open a new window. You don't need to navigate to any specific folder. Simply type the following command and hit Enter:

corebuddy

⌨️ Keybindings & Controls

corebuddy features full mouse support and terminal shortcuts for seamless navigation:
Left Click:	Select a specific CPU core to view its live telemetry report
r: Reset selection (Deselect the chosen core)
q: Safe exit from the application
Ctrl + C	Force close the application

📊 Technical Architecture

    Direct Kernel Telemetry: Reads straight from /proc/stat (CPU utilization), /proc/meminfo (RAM status), and /sys/class/thermal (CPU temperature) for near-zero CPU footprint.

    Responsive Layout Grid: Automatically computes terminal window rows and columns on resize, preventing layout breakage on smaller screens.

    Theme Palette: Built using the clean, high-contrast Catppuccin color palette via the lipgloss styling framework.

License

MIT License. Feel free to modify, fork, and strip down even further.

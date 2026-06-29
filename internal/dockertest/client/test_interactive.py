import pty
import os
import sys
import select
import time

def run_zsh_test(name, setup_cmds, test_cmds_and_expects):
    print(f"--- Running Test: {name} ---")
    pid, fd = pty.fork()
    if pid == 0:
        # Child process: set up zshrc and start zsh
        zshrc_content = "\n".join(setup_cmds) + "\n"
        with open(os.path.expanduser("~/.zshrc"), "w") as f:
            f.write(zshrc_content)
        
        # Start interactive zsh
        os.execvp("zsh", ["zsh", "-i"])
    else:
        # Parent process: interact with zsh
        buffer = b""
        
        # Helper to read until expected or timeout
        def expect(pattern, timeout=5):
            nonlocal buffer
            start = time.time()
            while time.time() - start < timeout:
                r, _, _ = select.select([fd], [], [], 0.05)
                if fd in r:
                    try:
                        data = os.read(fd, 1024)
                    except OSError:
                        break
                    if not data:
                        break
                    buffer += data
                    if pattern.encode() in buffer:
                        # Clear matched part of buffer
                        idx = buffer.index(pattern.encode()) + len(pattern)
                        buffer = buffer[idx:]
                        return True
            return False

        # Wait for initial prompt
        time.sleep(0.5)
        os.write(fd, b"\n")
        expect("%") # wait for zsh prompt

        for cmd, expected in test_cmds_and_expects:
            print(f"Sending: {cmd}")
            os.write(fd, cmd.encode() + b"\n")
            if not expect(expected):
                print(f"FAILED: Expected '{expected}' after '{cmd}'.\nRecent output: {buffer.decode(errors='replace')}")
                os.close(fd)
                sys.exit(1)
            print(f"PASSED: Found '{expected}'")
        
        # Clean up
        os.close(fd)
        print(f"Test {name} PASSED\n")

# Setup Zsh plugins in ~/.zshrc
vanilla_setup = [
    "# sssh Zsh Integration Hook",
    "sssh-accept-line() {",
    "    if sssh check-intercept \"$BUFFER\" \"$PWD\"; then",
    "        BUFFER=\"sssh $BUFFER\"",
    "    fi",
    "    zle sssh-orig-accept-line",
    "}",
    "() {",
    "    if [[ \"$widgets[accept-line]\" != \"user:sssh-accept-line\" ]]; then",
    "        local sssh_orig_widget=\"${widgets[accept-line]}\"",
    "        if [[ \"$sssh_orig_widget\" == *:* ]]; then",
    "            sssh_orig_widget=\"${sssh_orig_widget#*:}\"",
    "            zle -N sssh-orig-accept-line \"$sssh_orig_widget\"",
    "        else",
    "            sssh-orig-accept-line() {",
    "                zle .accept-line",
    "            }",
    "            zle -N sssh-orig-accept-line",
    "        fi",
    "        zle -N accept-line sssh-accept-line",
    "    fi",
    "}"
]

autosuggestions_setup = [
    "source /opt/zsh-autosuggestions/zsh-autosuggestions.zsh"
] + vanilla_setup

syntax_highlighting_setup = [
    "source /opt/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh"
] + vanilla_setup

multi_setup = [
    "source /opt/zsh-autosuggestions/zsh-autosuggestions.zsh",
    "source /opt/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh"
] + vanilla_setup

# Run Tests
try:
    # 1. Test Vanilla Zsh Hook
    run_zsh_test(
        "Vanilla Zsh",
        vanilla_setup,
        [
            ("echo local-test", "local-test"),
            ("hostname", "sssh-server"),
        ]
    )

    # 2. Test Autocomplete Chaining
    run_zsh_test(
        "Zsh + Autosuggestions",
        autosuggestions_setup,
        [
            ("echo hello-chain", "hello-chain"),
            ("hostname", "sssh-server"),
        ]
    )

    # 3. Test Syntax Highlighting
    run_zsh_test(
        "Zsh + Syntax Highlighting",
        syntax_highlighting_setup,
        [
            ("echo hello-syntax", "hello-syntax"),
            ("hostname", "sssh-server"),
        ]
    )

    # 4. Test Multi plugins + double source
    run_zsh_test(
        "Zsh + Multi + Double Source",
        multi_setup,
        [
            ("source ~/.zshrc", "%"),
            ("echo hello-multi", "hello-multi"),
            ("hostname", "sssh-server"),
        ]
    )

    # 5. Test Remove Pattern
    import subprocess
    print("Removing 'hostname' pattern...")
    subprocess.run(["sssh", "remove", "hostname"], check=True)
    
    run_zsh_test(
        "Vanilla Zsh Post-Remove",
        vanilla_setup,
        [
            ("hostname", "sssh-client"),
        ]
    )

    print("ALL INTERACTIVE SHELL TESTS COMPLETED SUCCESSFULLY")
    sys.exit(0)
except Exception as e:
    print(f"Error executing interactive shell tests: {e}")
    sys.exit(1)

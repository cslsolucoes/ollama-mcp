echo "== journal dir"; ls -ld /var/log/journal 2>&1; ls /var/log/journal 2>&1 | head; grep -vE '^\s*#|^\s*$' /etc/systemd/journald.conf /etc/systemd/journald.conf.d/*.conf 2>/dev/null
echo "== messages 12:00-12:09:40 (filtered)"; grep -E '^Oct 26 12:0[0-9]' /var/log/messages 2>/dev/null | grep -iE 'xid|aer|nvrm|uvm|pcie|reboot|shutdown|power|logind|watchdog|panic|thermal|oom|systemd\[1\]: (Stopp|Reach|Start).*(reboot|shutdown|poweroff)|halt|kexec|Stopping ollama|Received SIG|dnf|tuned|rpm' | tail -n 60
echo "== messages last 40 lines before boot"; awk '/^Oct 26 12:0[0-8]/' /var/log/messages 2>/dev/null | tail -n 40
echo "== secure 11:50-12:09"; grep -E '^Oct 26 1(1:5|2:0)' /var/log/secure 2>/dev/null | grep -viE 'publickey for claiton|session (opened|closed)' | tail -n 20
echo "== secure logins 11:50-12:09"; grep -E '^Oct 26 1(1:5|2:0)' /var/log/secure 2>/dev/null | grep -iE 'Accepted|sudo|reboot|shutdown' | tail -n 20
echo "== cron/timers"; grep -E '^Oct 26 1(1:5|2:0)' /var/log/cron 2>/dev/null | tail -n 10; systemctl list-timers --all --no-pager 2>/dev/null | head -n 20
echo "== dnf-automatic/needs-restarting"; systemctl is-enabled dnf-automatic.timer dnf-automatic-install.timer 2>&1; ls /etc/dnf/automatic.conf 2>/dev/null && grep -E '^(apply_updates|reboot)' /etc/dnf/automatic.conf
echo "== dnf history recent"; dnf history list 2>/dev/null | head -n 5
echo "== this boot kernel Xid/AER/NVRM"; journalctl -b 0 -k --no-pager -o short-iso 2>/dev/null | grep -iE 'xid|aer|nvrm|uvm|pcie bus error' | head -n 15
echo "== wtmp runlevel"; last -x -n 12 2>/dev/null | grep -E 'runlevel|shutdown|reboot' | head

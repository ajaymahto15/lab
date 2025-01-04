FROM ubuntu:20.04
EXPOSE 22
RUN apt-get update
RUN apt install -y net-tools dnsutils vim git openssh-server sudo ansible sshpass
RUN mkdir /var/run/sshd && chmod 0755 /var/run/sshd
RUN useradd -m -d /home/minion -s /bin/bash minion 
RUN echo 'minion:minion' | chpasswd
RUN usermod -aG sudo minion
RUN mkdir /home/minion/.ssh
RUN chmod 700 /home/minion/.ssh
RUN chown minion:minion /home/minion
CMD ["/usr/sbin/sshd", "-D"]

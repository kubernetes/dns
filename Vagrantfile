# -*- mode: ruby -*-
# vi: set ft=ruby :

# All Vagrant configuration is done below. The "2" in Vagrant.configure
# configures the configuration version (we support older styles for
# backwards compatibility). Please don't change it unless you know what
# you're doing.
Vagrant.configure("2") do |config|
  # The most common configuration options are documented and commented below.
  # For a complete reference, please see the online documentation at
  # https://docs.vagrantup.com.

  # Every Vagrant development environment requires a box. You can search for
  # boxes at https://atlas.hashicorp.com/search.
  config.vm.box = "ubuntu/xenial64"

  # Provider-specific configuration so you can fine-tune various
  # backing providers for Vagrant. These expose provider-specific options.
  # Example for VirtualBox:

  config.vm.provider "virtualbox" do |vb|
    # Customize the amount of memory on the VM:
    vb.memory = "4096"
  end

  # Set up Kube Env
  config.vm.provision "shell", inline: <<-SHELL
    apt-get -qq update
    apt-get -qq install -y apt-transport-https ca-certificates
    curl -fsSL https://yum.dockerproject.org/gpg | sudo apt-key add -
    echo "deb https://apt.dockerproject.org/repo ubuntu-xenial main" > /etc/apt/sources.list.d/docker.list
    apt-get -qq update
    apt-get -qq install -y linux-image-extra-$(uname -r)
    apt-get -qq install -y docker-engine rsync make
    wget --quiet https://storage.googleapis.com/golang/go1.7.5.linux-amd64.tar.gz
    tar -C /usr/local -xzf go1.7.5.linux-amd64.tar.gz
    echo export PATH=$PATH:/usr/local/go/bin | tee -a /etc/profile
    echo export GOPATH=/go | tee -a /etc/profile
  SHELL

  config.vm.provision "shell", inline: <<-SHELL
    echo export PATH=$PATH:/go/bin | tee -a /etc/profile
    go get github.com/jteeuwen/go-bindata/go-bindata github.com/golang/dep  
  SHELL

end

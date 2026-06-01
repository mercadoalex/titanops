# =============================================================================
# Packer Template: EKS Node with sched_ext kernel support
# Builds a custom AMI based on AL2023 with kernel 6.12+ for sched_ext
# Required by the Quack module for AI-powered CPU scheduling
# =============================================================================

packer {
  required_plugins {
    amazon = {
      version = ">= 1.2.0"
      source  = "github.com/hashicorp/amazon"
    }
  }
}

variable "aws_region" {
  type    = string
  default = "us-east-2"
}

variable "instance_type" {
  type    = string
  default = "m5.xlarge"
}

variable "ami_name_prefix" {
  type    = string
  default = "titanops-eks-scx-node"
}

variable "vpc_id" {
  type        = string
  description = "VPC ID to launch the build instance in. Required if no default VPC exists."
  default     = ""
}

variable "subnet_id" {
  type        = string
  description = "Subnet ID (public) to launch the build instance in."
  default     = ""
}

# Find the latest AL2023 EKS-optimized AMI as base
data "amazon-ami" "al2023_eks" {
  filters = {
    name                = "amazon-eks-node-al2023-x86_64-standard-1.32-*"
    virtualization-type = "hvm"
    root-device-type    = "ebs"
  }
  most_recent = true
  owners      = ["602401143452"] # Amazon EKS AMI account
  region      = var.aws_region
}

source "amazon-ebs" "scx_node" {
  ami_name      = "${var.ami_name_prefix}-{{timestamp}}"
  instance_type = var.instance_type
  region        = var.aws_region
  source_ami    = data.amazon-ami.al2023_eks.id

  vpc_id                      = var.vpc_id != "" ? var.vpc_id : null
  subnet_id                   = var.subnet_id != "" ? var.subnet_id : null
  associate_public_ip_address = true

  ssh_username = "ec2-user"

  ami_description = "TitanOps EKS node with kernel 6.12+ for sched_ext support (Quack module)"

  tags = {
    Name        = var.ami_name_prefix
    Project     = "titanops"
    Module      = "quack"
    Kernel      = "6.12+"
    SchedExt    = "enabled"
    ManagedBy   = "packer"
  }

  launch_block_device_mappings {
    device_name           = "/dev/xvda"
    volume_size           = 100
    volume_type           = "gp3"
    delete_on_termination = true
  }
}

build {
  sources = ["source.amazon-ebs.scx_node"]

  # Install kernel build dependencies
  provisioner "shell" {
    inline = [
      "sudo dnf install -y gcc make flex bison openssl-devel elfutils-libelf-devel bc perl dwarves ncurses-devel wget tar xz dracut",
      "sudo dnf install -y libbpf-devel bpftool",
    ]
  }

  # Download and compile kernel 6.12 with sched_ext enabled
  provisioner "shell" {
    inline = [
      "cd /tmp",
      "wget https://cdn.kernel.org/pub/linux/kernel/v6.x/linux-6.12.tar.xz",
      "tar xf linux-6.12.tar.xz",
      "cd linux-6.12",

      # Start from the current running config
      "cp /boot/config-$(uname -r) .config",

      # Enable sched_ext
      "scripts/config --enable CONFIG_SCHED_CLASS_EXT",
      "scripts/config --enable CONFIG_BPF",
      "scripts/config --enable CONFIG_BPF_SYSCALL",
      "scripts/config --enable CONFIG_BPF_JIT",
      "scripts/config --enable CONFIG_DEBUG_INFO_BTF",

      # Disable heavy debug info to save disk space
      "scripts/config --disable CONFIG_DEBUG_INFO_DWARF5",
      "scripts/config --disable CONFIG_DEBUG_INFO_DWARF_TOOLCHAIN_DEFAULT",
      "scripts/config --set-val CONFIG_DEBUG_INFO_REDUCED y",

      # Build the kernel (this takes ~20-30 min on m5.xlarge)
      "make olddefconfig",
      "make -j$(nproc)",
      "sudo make modules_install",

      # Manual kernel install (AL2023 uses grubby, not LILO)
      "sudo cp arch/x86/boot/bzImage /boot/vmlinuz-6.12.0",
      "sudo cp System.map /boot/System.map-6.12.0",
      "sudo cp .config /boot/config-6.12.0",
      "sudo dracut --force /boot/initramfs-6.12.0.img 6.12.0",

      # Add kernel entry to grub and set as default
      "sudo kernel-install add 6.12.0 /boot/vmlinuz-6.12.0 || sudo grubby --add-kernel=/boot/vmlinuz-6.12.0 --initrd=/boot/initramfs-6.12.0.img --title='Amazon Linux 6.12.0 (sched_ext)' --make-default",

      # Clean up build artifacts to reduce AMI size
      "cd /tmp && rm -rf linux-6.12*",
    ]
  }

  # Ensure BPF filesystem is mounted on boot
  provisioner "shell" {
    inline = [
      "echo 'bpf /sys/fs/bpf bpf defaults 0 0' | sudo tee -a /etc/fstab",
      "sudo mkdir -p /sys/fs/bpf",
    ]
  }

  # Install bpftool and verify
  provisioner "shell" {
    inline = [
      "sudo dnf install -y bpftool || true",
      "echo 'Custom AMI build complete. Kernel 6.12 with sched_ext will be active after reboot.'",
    ]
  }
}

class Logmind < Formula
  include Language::Python::Virtualenv

  desc "AI decision logging system for development projects"
  homepage "https://github.com/thrillmot/logmind"
  url "https://files.pythonhosted.org/packages/source/l/logmind/logmind-0.1.0.tar.gz"
  # sha256 will be updated when the package is published to PyPI
  sha256 "PLACEHOLDER_SHA256"
  license "MIT"
  head "https://github.com/thrillmot/logmind.git", branch: "main"

  depends_on "python@3.11"

  resource "click" do
    url "https://files.pythonhosted.org/packages/source/c/click/click-8.1.7.tar.gz"
    sha256 "ca9853ad459e787e2192211578cc907e7594e294c7ccc834310722b41b9ca6de"
  end

  resource "pyyaml" do
    url "https://files.pythonhosted.org/packages/source/P/PyYAML/PyYAML-6.0.1.tar.gz"
    sha256 "bfdf460b1736c775f2ba9f6a92bca30bc2095067b8a9d77876d1fad6cc3b4a43"
  end

  def install
    virtualenv_install_with_resources
  end

  test do
    system "#{bin}/logmind", "--version"
  end
end

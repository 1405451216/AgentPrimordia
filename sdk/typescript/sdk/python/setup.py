from setuptools import setup, find_packages

setup(
    name="agentprimordia",
    version="1.0.0",
    description="AgentPrimordia Universal AI Agent Development Framework",
    packages=find_packages(),
    python_requires=">=3.8",
    install_requires=[],
    extras_require={
        "dev": ["pytest", "black", "mypy"],
    },
)

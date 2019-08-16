pipeline {
  agent {
    kubernetes {
      defaultContainer 'jnlp'
      yaml """
apiVersion: v1
kind: Pod
metadata:
  annotations:
    kubernetes.io/target-runtime: kiyot
spec:
  containers:
  - name: golang
    image: golang:1.12
    command:
    - cat
    tty: true
    resources:
      requests:
        memory: "4Gi"
        cpu: "2000m"
      limits:
        memory: "4Gi"
        cpu: "2000m"
"""
    }
  }
  stages {
    stage('Build itzo') {
      steps {
        // Create symlink in GOPATH.
        container('golang') {
          sh 'mkdir -p /go/src/github.com/elotl; ln -s `pwd` /go/src/github.com/elotl/itzo'
        }
        // Build and release.
        container('golang') {
          sh 'cd /go/src/github.com/elotl/itzo && ./scripts/run_tests.sh'
        }
      }
    }
  }
}

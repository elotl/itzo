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
    image: elotl/golangbuild:latest
    command:
    - "/bin/sh"
    - "-c"
    - "sleep 10000"
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
  environment {
    AWS_DEFAULT_REGION    = 'us-east-1'
    AWS_REGION            = 'us-east-1'
    AWS_ACCESS_KEY_ID     = 'AKIA2BCIPONCV63FKG4J'
    AWS_SECRET_ACCESS_KEY = credentials('aws-ci-iam-secret-key')
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

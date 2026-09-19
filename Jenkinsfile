// Jenkins pipeline: build and push the ops backend (Go) and frontend (Vue) images.
//
// The tree module (github.com/gogu-x/tree) is resolved from the Go module proxy
// during `go build`, so only this repo needs to be checked out. The backend
// build context is server/, the frontend build context is frontend/.
//
// The Jenkins container has docker + git and mounts the host docker.sock, so
// `docker build` / `docker push` run against the host daemon directly.
//
// Credentials expected in Jenkins (create these before running, see setup notes):
//   - id 'dockerhub-xiaoguyun' : Username/Password credential for Docker Hub (only needed when PUSH=true)
//   - id 'github-ops'          : (optional) Git credential; not required while gogu-x/ops is public

pipeline {
    agent any

    parameters {
        string(name: 'OPS_BRANCH',       defaultValue: 'v1002',     description: 'Branch of gogu-x/ops to build')
        string(name: 'IMAGE_TAG',        defaultValue: '',          description: 'Image tag. Empty => <branch>-<short-sha>.')
        string(name: 'DOCKER_REGISTRY',  defaultValue: 'docker.io', description: 'Registry host')
        string(name: 'DOCKER_NAMESPACE', defaultValue: 'xiaoguyun', description: 'Registry namespace / Docker Hub user')
        booleanParam(name: 'PUSH', defaultValue: false, description: 'Push images to the registry after a successful build')
    }

    options {
        timestamps()
        disableConcurrentBuilds()
        timeout(time: 40, unit: 'MINUTES')
    }

    environment {
        DOCKERHUB_CREDENTIALS = 'dockerhub-xiaoguyun'  // Jenkins credential id (username/password)
        BACKEND_IMAGE_NAME    = 'gogs-ops-backend'
        FRONTEND_IMAGE_NAME   = 'gogs-ops-web'
    }

    stages {
        stage('Checkout') {
            steps {
                checkout([
                    $class: 'GitSCM',
                    branches: [[name: "*/${params.OPS_BRANCH}"]],
                    userRemoteConfigs: [[url: 'https://github.com/gogu-x/ops.git']]
                    // For a private repo, add: credentialsId: 'github-ops'
                ])
                script {
                    def sha = sh(script: 'git rev-parse --short HEAD', returnStdout: true).trim()
                    env.RESOLVED_TAG   = params.IMAGE_TAG?.trim() ? params.IMAGE_TAG.trim() : "${params.OPS_BRANCH}-${sha}"
                    env.BACKEND_IMAGE  = "${params.DOCKER_REGISTRY}/${params.DOCKER_NAMESPACE}/${env.BACKEND_IMAGE_NAME}:${env.RESOLVED_TAG}"
                    env.FRONTEND_IMAGE = "${params.DOCKER_REGISTRY}/${params.DOCKER_NAMESPACE}/${env.FRONTEND_IMAGE_NAME}:${env.RESOLVED_TAG}"
                    echo "Backend image : ${env.BACKEND_IMAGE}"
                    echo "Frontend image: ${env.FRONTEND_IMAGE}"
                }
            }
        }

        stage('Build backend') {
            steps {
                sh 'docker build -f server/Dockerfile -t "$BACKEND_IMAGE" server'
            }
        }

        stage('Build frontend') {
            steps {
                sh 'docker build -f frontend/Dockerfile -t "$FRONTEND_IMAGE" frontend'
            }
        }

        stage('Push') {
            when { expression { return params.PUSH } }
            steps {
                withCredentials([usernamePassword(
                    credentialsId: "${DOCKERHUB_CREDENTIALS}",
                    usernameVariable: 'REGISTRY_USER',
                    passwordVariable: 'REGISTRY_PASS'
                )]) {
                    sh '''
                        echo "$REGISTRY_PASS" | docker login "$DOCKER_REGISTRY" -u "$REGISTRY_USER" --password-stdin
                        docker push "$BACKEND_IMAGE"
                        docker push "$FRONTEND_IMAGE"
                        docker logout "$DOCKER_REGISTRY"
                    '''
                }
            }
        }
    }

    post {
        success {
            echo "Built ${env.BACKEND_IMAGE} and ${env.FRONTEND_IMAGE}" +
                 (params.PUSH ? " and pushed to ${params.DOCKER_REGISTRY}." : " (not pushed).")
        }
        always {
            sh 'docker image rm "$BACKEND_IMAGE" "$FRONTEND_IMAGE" 2>/dev/null || true'
        }
    }
}

pipeline {
  agent {
    node {
      label 'pagcloud'
    }
  }

  options {
    ansiColor('gnome-terminal')
  }

  environment {
    IMAGE_ID = "repo.intranet.pags/oracle-container-registry/database/observability-exporter"
  }

  stages("mkdkr") {
    stage("docker login") {
      steps {
          sh "make docker.login || true"
      }
    }

    stage("docker build") {
      steps {
          sh "make pags-docker-build TAGS=goora CGO_ENABLED=0 IMAGE_ID=${IMAGE_ID}"
      }
    }

    stage("docker push") {
      steps {
          sh "make pags-docker-push IMAGE_ID=${IMAGE_ID}"
      }
    }

  }

  }
}

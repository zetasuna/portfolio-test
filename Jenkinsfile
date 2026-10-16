pipeline {
  agent any
  options {
    timestamps()
    disableConcurrentBuilds()
    timeout(time: 60, unit: 'MINUTES')
    buildDiscarder(logRotator(numToKeepStr: '20', artifactNumToKeepStr: '10'))
  }
  environment {
    GO_IMAGE = 'golang:1.27.2-bookworm'
    SCANNER_IMAGE = 'aquasec/trivy:0.69.0'
    CURL_IMAGE = 'curlimages/curl:8.12.1'
    // IMAGE_TAG = ''
    // IMAGE_REPO = ''
    // HOST_WORKSPACE = ''
  }
  stages {
    stage('Checkout') {
      steps {
        checkout scm
        script {
          env.IMAGE_TAG = sh(
            returnStdout: true,
            script: 'git rev-parse --short=12 HEAD'
          ).trim()

          env.HOST_WORKSPACE = sh(
            returnStdout: true,
            script: '''
              set -eu
              container_id="$(hostname)"
              home_source="$(docker inspect \
                --format='{{range .Mounts}}{{if eq .Destination "/var/jenkins_home"}}{{.Source}}{{end}}{{end}}' \
                "$container_id")"
              test -n "$home_source"
              case "$WORKSPACE" in
                /var/jenkins_home/*)
                  relative="${WORKSPACE#/var/jenkins_home/}"
                  ;;
                *)
                  echo "Unexpected workspace: $WORKSPACE" >&2
                  exit 1
                  ;;
              esac
              printf '%s/%s' "$home_source" "$relative"
            '''
          ).trim()

          if (!env.HOST_WORKSPACE?.trim()) {
              error('HOST_WORKSPACE is empty; check Jenkins Docker volume mapping.')
          }

          echo "IMAGE_TAG=${env.IMAGE_TAG}"
          echo "HOST_WORKSPACE=${env.HOST_WORKSPACE}"
        }
      }
    }
    stage('Format, vet and unit tests') {
      steps {
        sh '''
          set -eu
          docker run --rm -v "$HOST_WORKSPACE:/src" -w /src "$GO_IMAGE" sh -ec '
            go mod tidy
            test -z "$(gofmt -l cmd internal tests)" || { gofmt -l cmd internal tests; exit 1; }
            go vet ./...
            go test ./... -race -coverprofile=coverage.out
            go tool cover -func=coverage.out | tail -n 1
          '
        '''
        archiveArtifacts artifacts: 'coverage.out', allowEmptyArchive: true
      }
    }
    stage('Database integration tests') {
      // when { branch 'main' }
      steps {
        withCredentials([file(credentialsId: 'portfolio-runtime-env', variable: 'RUNTIME_ENV')]) {
          sh '''
            echo "Hello"
          '''
          // sh '''
          //   set +x
          //   set -eu
          //   set -a
          //   . "$RUNTIME_ENV"
          //   set +a
          //   TEST_DATABASE_URL="postgres://${APP_DB_USER}:${APP_DB_PASSWORD}@postgres:5432/${TEST_DB_NAME}?sslmode=disable"
          //   TEST_CONTAINER="portfolio-integration-${BUILD_NUMBER}"
          //   docker rm -f "$TEST_CONTAINER" >/dev/null 2>&1 || true
          //   cleanup() { docker rm -f "$TEST_CONTAINER" >/dev/null 2>&1 || true; }
          //   trap cleanup EXIT
          //   # Keep the default bridge for module downloads, then attach the internal DB network.
          //   docker create --name "$TEST_CONTAINER" --network bridge \
          //     -v "$HOST_WORKSPACE:/src" -w /src \
          //     -e TEST_DATABASE_URL="$TEST_DATABASE_URL" -e MIGRATIONS_DIR=/src/migrations \
          //     "$GO_IMAGE" sh -ec 'go mod download && go test ./tests/integration -v' >/dev/null
          //   docker network connect portfolio-crud_db_net "$TEST_CONTAINER"
          //   docker start --attach "$TEST_CONTAINER"
          // '''
        }
      }
    }
    stage('Dependency vulnerability scan') {
      steps {
        sh '''
          echo "Hello"
        '''
        // sh '''
        //   set -eu
        //   docker run --rm -v "$HOST_WORKSPACE:/src" -w /src "$GO_IMAGE" sh -ec \
        //     'go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...'
        // '''
      }
    }
    stage('Build image') {
      steps {
        sh '''
          set -eu
          DOCKER_BUILDKIT=1 docker build --pull --tag portfolio-crud:scan .
        '''
      }
    }
    stage('Container image vulnerability scan') {
      steps {
        sh '''
          set -eu
          docker run --rm -v /var/run/docker.sock:/var/run/docker.sock "$SCANNER_IMAGE" \
            image --scanners vuln --severity HIGH,CRITICAL --exit-code 0 portfolio-crud:scan
        '''
      }
    }
    stage('Publish trusted image') {
      // when { anyOf { branch 'main'; buildingTag() } }
      steps {
        withCredentials([
          usernamePassword(
            credentialsId: 'dockerhub',
            usernameVariable: 'DOCKERHUB_USER',
            passwordVariable: 'DOCKERHUB_TOKEN'
          )
        ]) {
          sh '''
            set +x
            set -eu
            test -n "$DOCKERHUB_USER"
            IMAGE_REPO="$DOCKERHUB_USER/portfolio"
            echo "$DOCKERHUB_TOKEN" | docker login --username "$DOCKERHUB_USER" --password-stdin
            docker tag portfolio-crud:scan "$IMAGE_REPO:$IMAGE_TAG"
            docker push "$IMAGE_REPO:$IMAGE_TAG"
            docker logout
            printf 'APP_IMAGE=%s:%s\n' "$IMAGE_REPO" "$IMAGE_TAG" > release.env
          '''
          archiveArtifacts artifacts: 'release.env', fingerprint: true
        }
      }
    }
    stage('Deploy and smoke test') {
      // when { anyOf { branch 'main'; buildingTag() } }
      steps {
        withCredentials([file(credentialsId: 'portfolio-runtime-env', variable: 'RUNTIME_ENV')]) {
          sh '''
            set +x
            set -eu
            PREVIOUS_IMAGE="$(docker ps --filter label=com.docker.compose.project=portfolio-crud --filter label=com.docker.compose.service=app1 --format '{{.Image}}' | head -n 1 || true)"
            test -s release.env
            docker compose \
              --project-name portfolio-crud \
              --env-file "$RUNTIME_ENV" --env-file "$WORKSPACE/release.env" \
              -f "$WORKSPACE/compose.yaml" up -d --no-build --pull always
            if docker run --rm --network host -v "$HOST_WORKSPACE:/src:ro" -w /src -e BASE_URL=http://127.0.0.1:8080 --entrypoint /bin/sh "$CURL_IMAGE" -ec './scripts/smoke.sh'; then
              echo "Smoke test passed for $IMAGE_TAG"
            else
              echo "Smoke test failed; attempting rollback" >&2
              if [ -n "$PREVIOUS_IMAGE" ]; then
                printf 'APP_IMAGE=%s\n' "$PREVIOUS_IMAGE" > "$WORKSPACE/release.env"
                docker compose --project-name portfolio-crud --env-file "$RUNTIME_ENV" --env-file "$WORKSPACE/release.env" -f "$WORKSPACE/compose.yaml" up -d --no-build
                docker run --rm --network host -v "$HOST_WORKSPACE:/src:ro" -w /src -e BASE_URL=http://127.0.0.1:8080 --entrypoint /bin/sh "$CURL_IMAGE" -ec './scripts/smoke.sh' || true
              else
                echo "No previous app image was found; manual recovery is required" >&2
              fi
              exit 1
            fi
          '''
        }
      }
    }
  }
  post {
    always {
      sh 'docker logout >/dev/null 2>&1 || true'
      deleteDir()
    }
  }
}

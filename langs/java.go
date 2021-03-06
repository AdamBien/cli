package langs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// JavaLangHelper provides a set of helper methods for the lifecycle of Java Maven projects
type JavaLangHelper struct {
	BaseHelper
	version string
}

// BuildFromImage returns the Docker image used to compile the Maven function project
func (lh *JavaLangHelper) BuildFromImage() string {
	if lh.version == "1.8" {
		return "fnproject/fn-java-fdk-build:latest"
	} else if lh.version == "9" {
		return "fnproject/fn-java-fdk-build:jdk9-latest"
	} else {
		return ""
	}
}

// RunFromImage returns the Docker image used to run the Java function.
func (lh *JavaLangHelper) RunFromImage() string {
	if lh.version == "1.8" {
		return "fnproject/fn-java-fdk:latest"
	} else if lh.version == "9" {
		return "fnproject/fn-java-fdk:jdk9-latest"
	} else {
		return ""
	}
}

// HasBoilerplate returns whether the Java runtime has boilerplate that can be generated.
func (lh *JavaLangHelper) HasBoilerplate() bool { return true }

// GenerateBoilerplate will generate function boilerplate for a Java runtime. The default boilerplate is for a Maven
// project.
func (lh *JavaLangHelper) GenerateBoilerplate() error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	pathToPomFile := filepath.Join(wd, "pom.xml")
	if exists(pathToPomFile) {
		return ErrBoilerplateExists
	}

	apiVersion, err := getFDKAPIVersion()
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(pathToPomFile, []byte(pomFileContent(apiVersion, lh.version)), os.FileMode(0644)); err != nil {
		return err
	}

	mkDirAndWriteFile := func(dir, filename, content string) error {
		fullPath := filepath.Join(wd, dir)
		if err = os.MkdirAll(fullPath, os.FileMode(0755)); err != nil {
			return err
		}

		fullFilePath := filepath.Join(fullPath, filename)
		return ioutil.WriteFile(fullFilePath, []byte(content), os.FileMode(0644))
	}

	err = mkDirAndWriteFile("src/main/java/com/example/fn", "HelloFunction.java", helloJavaSrcBoilerplate)
	if err != nil {
		return err
	}

	return mkDirAndWriteFile("src/test/java/com/example/fn", "HelloFunctionTest.java", helloJavaTestBoilerplate)
}

// Cmd returns the Java runtime Docker entrypoint that will be executed when the function is executed.
func (lh *JavaLangHelper) Cmd() string {
	return "com.example.fn.HelloFunction::handleRequest"
}

// DockerfileCopyCmds returns the Docker COPY command to copy the compiled Java function jar and dependencies.
func (lh *JavaLangHelper) DockerfileCopyCmds() []string {
	return []string{
		"COPY --from=build-stage /function/target/*.jar /function/app/",
	}
}

// DockerfileBuildCmds returns the build stage steps to compile the Maven function project.
func (lh *JavaLangHelper) DockerfileBuildCmds() []string {
	return []string{
		fmt.Sprintf("ENV MAVEN_OPTS %s", mavenOpts()),
		"ADD pom.xml /function/pom.xml",
		"RUN [\"mvn\", \"package\", \"dependency:copy-dependencies\", \"-DincludeScope=runtime\", " +
			"\"-DskipTests=true\", \"-Dmdep.prependGroupId=true\", \"-DoutputDirectory=target\", \"--fail-never\"]",
		"ADD src /function/src",
		"RUN [\"mvn\", \"package\"]",
	}
}

// HasPreBuild returns whether the Java Maven runtime has a pre-build step.
func (lh *JavaLangHelper) HasPreBuild() bool { return true }

// PreBuild ensures that the expected the function is based is a maven project.
func (lh *JavaLangHelper) PreBuild() error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	if !exists(filepath.Join(wd, "pom.xml")) {
		return errors.New("Could not find pom.xml - are you sure this is a Maven project?")
	}

	return nil
}

func mavenOpts() string {
	var opts bytes.Buffer

	if parsedURL, err := url.Parse(os.Getenv("http_proxy")); err == nil {
		opts.WriteString(fmt.Sprintf("-Dhttp.proxyHost=%s ", parsedURL.Hostname()))
		opts.WriteString(fmt.Sprintf("-Dhttp.proxyPort=%s ", parsedURL.Port()))
	}

	if parsedURL, err := url.Parse(os.Getenv("https_proxy")); err == nil {
		opts.WriteString(fmt.Sprintf("-Dhttps.proxyHost=%s ", parsedURL.Hostname()))
		opts.WriteString(fmt.Sprintf("-Dhttps.proxyPort=%s ", parsedURL.Port()))
	}

	nonProxyHost := os.Getenv("no_proxy")
	opts.WriteString(fmt.Sprintf("-Dhttp.nonProxyHosts=%s ", strings.Replace(nonProxyHost, ",", "|", -1)))

	opts.WriteString("-Dmaven.repo.local=/usr/share/maven/ref/repository")

	return opts.String()
}

/*    TODO temporarily generate maven project boilerplate from hardcoded values.
Will eventually move to using a maven archetype.
*/
func pomFileContent(APIversion, javaVersion string) string {
	return fmt.Sprintf(pomFile, APIversion, APIversion, javaVersion, javaVersion)
}

func getFDKAPIVersion() (string, error) {
	const versionURL = "https://api.bintray.com/search/packages/maven?repo=fnproject&g=com.fnproject.fn&a=fdk"
	const versionEnv = "FN_JAVA_FDK_VERSION"
	fetchError := fmt.Errorf("Failed to fetch latest Java FDK version from %v. Check your network settings or manually override the version by setting %s", versionURL, versionEnv)

	type parsedResponse struct {
		Version string `json:"latest_version"`
	}
	version := os.Getenv(versionEnv)
	if version != "" {
		return version, nil
	}
	resp, err := http.Get(versionURL)
	if err != nil || resp.StatusCode != 200 {
		return "", fetchError
	}

	buf := bytes.Buffer{}
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		return "", fetchError
	}

	parsedResp := make([]parsedResponse, 1)
	err = json.Unmarshal(buf.Bytes(), &parsedResp)
	if err != nil {
		return "", fetchError
	}
	return parsedResp[0].Version, nil
}

const (
	pomFile = `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example.fn</groupId>
    <artifactId>hello</artifactId>
    <version>1.0.0</version>
    <repositories>
        <repository>
            <id>fn-release-repo</id>
            <url>https://dl.bintray.com/fnproject/fnproject</url>
            <releases>
                <enabled>true</enabled>
            </releases>
            <snapshots>
                <enabled>false</enabled>
            </snapshots>
        </repository>
    </repositories>

    <dependencies>
        <dependency>
            <groupId>com.fnproject.fn</groupId>
            <artifactId>api</artifactId>
            <version>%s</version>
        </dependency>
        <dependency>
            <groupId>com.fnproject.fn</groupId>
            <artifactId>testing</artifactId>
            <version>%s</version>
            <scope>test</scope>
        </dependency>
        <dependency>
            <groupId>junit</groupId>
            <artifactId>junit</artifactId>
            <version>4.12</version>
            <scope>test</scope>
        </dependency>
    </dependencies>
    <properties>
		<maven.compiler.source>%s</maven.compiler.source>
		<maven.compiler.target>%s</maven.compiler.target>
        <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>		
	</properties>	
</project>
`

	helloJavaSrcBoilerplate = `package com.example.fn;

public class HelloFunction {

    public String handleRequest(String input) {
        String name = (input == null || input.isEmpty()) ? "world"  : input;

        return "Hello, " + name + "!";
    }

}`

	helloJavaTestBoilerplate = `package com.example.fn;

import com.fnproject.fn.testing.*;
import org.junit.*;

import static org.junit.Assert.*;

public class HelloFunctionTest {

    @Rule
    public final FnTestingRule testing = FnTestingRule.createDefault();

    @Test
    public void shouldReturnGreeting() {
        testing.givenEvent().enqueue();
        testing.thenRun(HelloFunction.class, "handleRequest");

        FnResult result = testing.getOnlyResult();
        assertEquals("Hello, world!", result.getBodyAsString());
    }

}`
)

const { events } = require("brigadier");
const {
  TestingJobGenerator,
  NotificationJobGenerator,
  BuildStatus
} = require("./spotahome");

const buildStatusSuccess = "success";

const eventTypePush = "push";
const eventTypeExec = "exec";
const eventTypePullRequest = "pull_request";
const eventTypeAfter = "after";
const eventTypeError = "error";

function getUnitTestsJob(e, project) {
  let tjg = new TestingJobGenerator(e, project);
  return tjg.golangModules("make ci");
}

/**
 * testBuild only runs tests. useful for pull requests.
 */
function testBuild(e, project) {
  let unitTestsJob = getUnitTestsJob();

  let njg = new NotificationJobGenerator(e, project);
  njg.githubSetStatus(BuildStatus.Pending).run();

  unitTestsJob.run().then(() => {
    njg.githubSetStatus(BuildStatus.Success).run();
  });
}

/**
 * errorToGithub will put the build error status on github.
 */
function errorToGithub(e, project) {
  let njg = new NotificationJobGenerator(e, project);
  njg.githubSetStatus(BuildStatus.Failure).run();
}

events.on(eventTypePush, testBuild);
events.on(eventTypeExec, testBuild);
events.on(eventTypePullRequest, testBuild);

// Final events after build (failure).
events.on(eventTypeError, errorToGithub);

import { spawn, execSync } from 'child_process';
import { existsSync } from 'fs';
import { homedir } from 'os';
import { join } from 'path';
import chalk from 'chalk';

const INSTALL_DIR = join(homedir(), '.sonos-http-api');
const REPO_URL = 'https://github.com/jishi/node-sonos-http-api.git';

export async function runSetup(): Promise<void> {
  console.log(chalk.blue('Setting up node-sonos-http-api...\n'));

  // Check if already installed
  if (existsSync(INSTALL_DIR)) {
    console.log(chalk.yellow(`Found existing installation at ${INSTALL_DIR}`));
    console.log(chalk.blue('Pulling latest changes...'));
    try {
      execSync('git pull', { cwd: INSTALL_DIR, stdio: 'inherit' });
    } catch {
      console.log(chalk.yellow('Could not pull updates, continuing...'));
    }
  } else {
    // Clone repository
    console.log(chalk.blue(`Cloning to ${INSTALL_DIR}...`));
    try {
      execSync(`git clone ${REPO_URL} "${INSTALL_DIR}"`, { stdio: 'inherit' });
    } catch (error) {
      console.error(chalk.red('Failed to clone repository'));
      console.error(chalk.red('Make sure git is installed'));
      process.exit(1);
    }
  }

  // Install dependencies
  console.log(chalk.blue('\nInstalling dependencies...'));
  try {
    execSync('npm install', { cwd: INSTALL_DIR, stdio: 'inherit' });
  } catch (error) {
    console.error(chalk.red('Failed to install dependencies'));
    process.exit(1);
  }

  // Start the service
  console.log(chalk.blue('\nStarting node-sonos-http-api...'));
  console.log(chalk.gray('(Press Ctrl+C to stop)\n'));

  const child = spawn('node', ['server.js'], {
    cwd: INSTALL_DIR,
    stdio: 'inherit',
  });

  child.on('error', (error) => {
    console.error(chalk.red(`Failed to start: ${error.message}`));
    process.exit(1);
  });

  child.on('exit', (code) => {
    if (code !== 0) {
      console.error(chalk.red(`Process exited with code ${code}`));
    }
    process.exit(code || 0);
  });

  // Handle Ctrl+C
  process.on('SIGINT', () => {
    console.log(chalk.yellow('\nStopping...'));
    child.kill('SIGINT');
  });
}

export function printSetupInstructions(): void {
  console.log(chalk.red('Could not connect to node-sonos-http-api\n'));
  console.log(chalk.white('The Sonos HTTP API service needs to be running.'));
  console.log(chalk.white('Run the following command to set it up:\n'));
  console.log(chalk.cyan('  sonos --setup\n'));
  console.log(
    chalk.white('Or start it manually if already installed:')
  );
  console.log(chalk.cyan(`  cd ${INSTALL_DIR} && node server.js\n`));
}

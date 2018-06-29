pull images
# Create app directory
RUN mkdir /home/Service
WORKDIR - p /home/Service
# Bundle app source
COPY . /home/Service
RUN npm install
CMD [ "npm", "start"]


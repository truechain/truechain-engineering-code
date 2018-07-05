FROM images
# Create app directory
RUN mkdir /home/Service
WORKDIR -cp /home/Service
# Bundle app source
COPY . /home/Service
RUN npm install
CMD [ "npm", "start"]

